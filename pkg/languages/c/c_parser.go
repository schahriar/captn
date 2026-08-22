package languages_c

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/lsp"
	"github.com/schahriar/captn/pkg/parsers"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_c "github.com/tree-sitter/tree-sitter-c/bindings/go"
	tree_sitter_cpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"
)

// hasSourceWidth reports whether a node spans any bytes; tree-sitter error
// recovery produces zero-width nodes that must never become AST nodes (the
// interval index rejects empty ranges)
func hasSourceWidth(node parsers.ParserNode) bool {
	return node.Range.StartByte < node.Range.EndByte
}

func collectIdentifiers(node parsers.ParserNode) []*ast.ASTSymbol {
	if node.Kind == "identifier" || node.Kind == "field_identifier" {
		if !hasSourceWidth(node) {
			return nil
		}
		return []*ast.ASTSymbol{ast.NewASTSymbol(ast.NewASTNodeContainer(node), node.GetTextContent())}
	}
	var syms []*ast.ASTSymbol
	node.IterateChildren(func(child parsers.ParserNode) (bool, error) {
		syms = append(syms, collectIdentifiers(child)...)
		return true, nil
	})
	return syms
}

// childDeclarator steps into the declarator a wrapper nests; parenthesized
// and reference declarators carry no field for it
func childDeclarator(node parsers.ParserNode) (parsers.ParserNode, bool) {
	if inner, ok := node.ChildByFieldName("declarator"); ok {
		return inner, true
	}

	var found parsers.ParserNode
	ok := false

	node.IterateChildren(func(child parsers.ParserNode) (bool, error) {
		found = child
		ok = true
		return false, nil
	})

	return found, ok
}

// findFunctionDeclarator reports the function_declarator a declarator chain
// carries. One wrapping a parenthesized declarator is a function-pointer
// variable, not a function
func findFunctionDeclarator(node parsers.ParserNode) (parsers.ParserNode, bool) {
	switch node.Kind {
	case "function_declarator":
		if inner, ok := childDeclarator(node); ok && inner.Kind == "parenthesized_declarator" {
			var zero parsers.ParserNode
			return zero, false
		}

		return node, true

	case "pointer_declarator", "reference_declarator", "init_declarator", "attributed_declarator":
		if inner, ok := childDeclarator(node); ok {
			return findFunctionDeclarator(inner)
		}
	}

	var zero parsers.ParserNode
	return zero, false
}

// declaredName descends a declarator or qualified name to the node the
// declaration names; the rightmost name of a qualified identifier is the
// declared one
func declaredName(node parsers.ParserNode) (parsers.ParserNode, bool) {
	switch node.Kind {
	case "identifier", "field_identifier", "type_identifier", "namespace_identifier",
		"operator_name", "operator_cast", "structured_binding_declarator":
		return node, hasSourceWidth(node)

	case "destructor_name":
		if ident, ok := node.GetNthChildByKind("identifier", 0); ok && hasSourceWidth(ident) {
			return ident, true
		}

		return node, hasSourceWidth(node)

	case "qualified_identifier", "template_function", "template_method", "template_type":
		if inner, ok := node.ChildByFieldName("name"); ok {
			return declaredName(inner)
		}

	case "pointer_declarator", "array_declarator", "init_declarator", "function_declarator",
		"reference_declarator", "parenthesized_declarator", "attributed_declarator", "variadic_declarator":
		if inner, ok := childDeclarator(node); ok {
			return declaredName(inner)
		}
	}

	var zero parsers.ParserNode
	return zero, false
}

func declaredSymbol(node parsers.ParserNode) *ast.ASTSymbol {
	nameNode, ok := declaredName(node)

	if !ok || nameNode.Kind == "structured_binding_declarator" {
		return nil
	}

	return ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
}

// typeExpression maps a type onto the identifier a definition resolves to.
// Primitive and sized types are keywords with no definition site and yield
// nothing. Nameless composites (pointers, arrays, function types) fold flat:
// the first named type inside is the head and the rest are its arguments.
func typeExpression(node parsers.ParserNode) *ast.ASTTypeExpression {
	switch node.Kind {
	case "type_identifier":
		if !hasSourceWidth(node) {
			return nil
		}

		return ast.NewASTTypeExpression(ast.NewASTNodeContainer(node), node.GetTextContent())

	case "qualified_identifier":
		nameNode, ok := node.ChildByFieldName("name")

		if !ok {
			return nil
		}

		var texpr *ast.ASTTypeExpression

		switch nameNode.Kind {
		case "qualified_identifier", "template_type":
			texpr = typeExpression(nameNode)
		default:
			if !hasSourceWidth(nameNode) {
				return nil
			}

			texpr = ast.NewASTTypeExpression(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
		}

		if texpr == nil {
			return nil
		}

		if scopeNode, ok := node.ChildByFieldName("scope"); ok && scopeNode.Kind == "namespace_identifier" && hasSourceWidth(scopeNode) {
			texpr.Namespace = ast.NewASTSymbol(ast.NewASTNodeContainer(scopeNode), scopeNode.GetTextContent())
		}

		return texpr

	case "template_type":
		headNode, ok := node.ChildByFieldName("name")

		if !ok {
			return nil
		}

		head := typeExpression(headNode)

		if head == nil {
			return nil
		}

		if argsNode, ok := node.ChildByFieldName("arguments"); ok {
			head.Arguments = append(head.Arguments, collectTypes(argsNode)...)
		}

		return head

	case "struct_specifier", "union_specifier", "enum_specifier", "class_specifier":
		// Bodiless is a reference like `struct Widget *w`; a bodied
		// specifier is a definition the transformer maps on its own
		if _, ok := node.ChildByFieldName("body"); ok {
			return nil
		}

		if nameNode, ok := node.ChildByFieldName("name"); ok {
			return typeExpression(nameNode)
		}

		return nil

	case "primitive_type", "sized_type_specifier", "placeholder_type_specifier", "auto":
		return nil
	}

	types := collectTypes(node)

	if len(types) == 0 {
		return nil
	}

	types[0].Arguments = append(types[0].Arguments, types[1:]...)

	return types[0]
}

func collectTypes(node parsers.ParserNode) []*ast.ASTTypeExpression {
	var types []*ast.ASTTypeExpression

	node.IterateChildren(func(child parsers.ParserNode) (bool, error) {
		switch child.Kind {
		case "type_identifier", "qualified_identifier", "template_type",
			"struct_specifier", "union_specifier", "enum_specifier", "class_specifier":
			if texpr := typeExpression(child); texpr != nil {
				types = append(types, texpr)
			}
		default:
			types = append(types, collectTypes(child)...)
		}

		return true, nil
	})

	return types
}

func appendParameter(fn *ast.ASTFuncExpression, decl parsers.ParserNode) {
	var texpr *ast.ASTTypeExpression

	if typeNode, ok := decl.ChildByFieldName("type"); ok {
		texpr = typeExpression(typeNode)
	}

	var sym *ast.ASTSymbol

	if declNode, ok := decl.ChildByFieldName("declarator"); ok {
		sym = declaredSymbol(declNode)
	}

	fn.Arguments = append(fn.Arguments, ast.NewASTFuncArgument(ast.NewASTNodeContainer(decl), sym, texpr))
}

func appendParameters(fn *ast.ASTFuncExpression, fdecl parsers.ParserNode) {
	list, ok := fdecl.ChildByFieldName("parameters")

	if !ok {
		return
	}

	list.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
		switch pn.Kind {
		case "parameter_declaration", "optional_parameter_declaration", "variadic_parameter_declaration":
			appendParameter(fn, pn)
		}
		return true, nil
	})
}

func appendTemplateParameters(fn *ast.ASTFuncExpression, list parsers.ParserNode) {
	list.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
		switch pn.Kind {
		case "type_parameter_declaration", "variadic_type_parameter_declaration",
			"template_template_parameter_declaration":
			if nameNode, ok := pn.GetNthChildByKind("type_identifier", 0); ok && hasSourceWidth(nameNode) {
				sym := ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
				fn.Arguments = append(fn.Arguments, ast.NewASTFuncArgument(ast.NewASTNodeContainer(pn), sym, nil))
			}

		case "optional_type_parameter_declaration":
			var sym *ast.ASTSymbol

			if nameNode, ok := pn.ChildByFieldName("name"); ok && hasSourceWidth(nameNode) {
				sym = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
			}

			fn.Arguments = append(fn.Arguments, ast.NewASTFuncArgument(ast.NewASTNodeContainer(pn), sym, nil))

		case "parameter_declaration", "optional_parameter_declaration":
			appendParameter(fn, pn)
		}

		return true, nil
	})
}

// emitFunction maps a function_definition or a function-shaped declaration
// (a prototype). The container may be the wrapping template_declaration so a
// snippet starting at `template<...>` roots at the function; a prototype has
// no body to narrow to and keeps its block shadow deliberately
func emitFunction(ctx context.Context, trx *parsers.TransformContext, container parsers.ParserNode, node parsers.ParserNode, templates parsers.ParserNode, hasTemplates bool) error {
	declNode, ok := node.ChildByFieldName("declarator")

	if !ok {
		return trx.WalkChildren(ctx)
	}

	fdecl, ok := findFunctionDeclarator(declNode)

	if !ok {
		return trx.WalkChildren(ctx)
	}

	fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(container))

	if inner, ok := childDeclarator(fdecl); ok {
		fn.Name = declaredSymbol(inner)
	}

	if hasTemplates {
		appendTemplateParameters(fn, templates)
	}

	appendParameters(fn, fdecl)

	if typeNode, ok := node.ChildByFieldName("type"); ok {
		fn.ReturnType = typeExpression(typeNode)
	}

	if fn.ReturnType == nil {
		if trailing, ok := fdecl.GetNthChildByKind("trailing_return_type", 0); ok {
			fn.ReturnType = typeExpression(trailing)
		}
	}

	if bodyNode, ok := node.ChildByFieldName("body"); ok {
		fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(bodyNode))
	}

	if err := trx.Emit(fn); err != nil {
		return err
	}

	return trx.WalkChildrenInto(ctx, fn)
}

// emitType maps a bodied struct, union, class or enum specifier; members map
// through the walk the way class bodies need (inline methods carry code)
func emitType(ctx context.Context, trx *parsers.TransformContext, container parsers.ParserNode, spec parsers.ParserNode, templates parsers.ParserNode, hasTemplates bool) error {
	fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(container))

	if nameNode, ok := spec.ChildByFieldName("name"); ok {
		fn.Name = declaredSymbol(nameNode)
	}

	if hasTemplates {
		appendTemplateParameters(fn, templates)
	}

	if bodyNode, ok := spec.ChildByFieldName("body"); ok {
		fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(bodyNode))
	}

	if clause, ok := spec.GetNthChildByKind("base_class_clause", 0); ok {
		for _, texpr := range collectTypes(clause) {
			fn.AppendChild(texpr)
		}
	}

	if err := trx.Emit(fn); err != nil {
		return err
	}

	return trx.WalkChildrenInto(ctx, fn)
}

func emitDeclaration(ctx context.Context, trx *parsers.TransformContext, node parsers.ParserNode) error {
	var declarators []parsers.ParserNode

	node.IterateChildrenByFieldName("declarator", func(pn parsers.ParserNode) (bool, error) {
		declarators = append(declarators, pn)
		return true, nil
	})

	if len(declarators) == 1 {
		if _, ok := findFunctionDeclarator(declarators[0]); ok {
			var zero parsers.ParserNode
			return emitFunction(ctx, trx, node, node, zero, false)
		}
	}

	decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(node))

	for _, pn := range declarators {
		nameNode, ok := declaredName(pn)

		if !ok {
			continue
		}

		if nameNode.Kind == "structured_binding_declarator" {
			decl.Names = append(decl.Names, collectIdentifiers(nameNode)...)
			continue
		}

		decl.Names = append(decl.Names, ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent()))
	}

	if typeNode, ok := node.ChildByFieldName("type"); ok {
		decl.Type = typeExpression(typeNode)
	}

	if len(decl.Names) == 0 && decl.Type == nil {
		return trx.WalkChildren(ctx)
	}

	if err := trx.Emit(decl); err != nil {
		return err
	}

	// RHS: walk into decl so calls and lambdas in initializers land in Virtual
	return trx.WalkChildrenInto(ctx, decl)
}

// emitTypedef maps `typedef struct {...} Name` onto one type vertex; a
// memberless typedef is a plain declaration. Both the tag and the typedef
// names are definition sites, so every spelling keeps a node
func emitTypedef(ctx context.Context, trx *parsers.TransformContext, node parsers.ParserNode) error {
	typeNode, ok := node.ChildByFieldName("type")

	if !ok {
		return trx.WalkChildren(ctx)
	}

	var names []parsers.ParserNode

	node.IterateChildrenByFieldName("declarator", func(pn parsers.ParserNode) (bool, error) {
		if nameNode, ok := declaredName(pn); ok && nameNode.Kind != "structured_binding_declarator" {
			names = append(names, nameNode)
		}
		return true, nil
	})

	bodied := false

	switch typeNode.Kind {
	case "struct_specifier", "union_specifier", "enum_specifier", "class_specifier":
		_, bodied = typeNode.ChildByFieldName("body")
	}

	if !bodied {
		decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(node))

		for _, nameNode := range names {
			decl.Names = append(decl.Names, ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent()))
		}

		decl.Type = typeExpression(typeNode)

		if len(decl.Names) == 0 && decl.Type == nil {
			return trx.WalkChildren(ctx)
		}

		return trx.Emit(decl)
	}

	fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))

	if nameNode, ok := typeNode.ChildByFieldName("name"); ok {
		fn.Name = declaredSymbol(nameNode)
	}

	if fn.Name == nil && len(names) > 0 {
		fn.Name = ast.NewASTSymbol(ast.NewASTNodeContainer(names[0]), names[0].GetTextContent())
		names = names[1:]
	}

	if bodyNode, ok := typeNode.ChildByFieldName("body"); ok {
		fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(bodyNode))
	}

	if err := trx.Emit(fn); err != nil {
		return err
	}

	for _, nameNode := range names {
		fn.AppendChild(ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent()))
	}

	return trx.WalkChildrenInto(ctx, fn)
}

func emitTemplateAlias(trx *parsers.TransformContext, container parsers.ParserNode, node parsers.ParserNode, templates parsers.ParserNode, hasTemplates bool) error {
	fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(container))

	if nameNode, ok := node.ChildByFieldName("name"); ok && hasSourceWidth(nameNode) {
		fn.Name = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
	}

	if hasTemplates {
		appendTemplateParameters(fn, templates)
	}

	if typeNode, ok := node.ChildByFieldName("type"); ok {
		fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(typeNode))

		if texpr := typeExpression(typeNode); texpr != nil {
			fn.AppendChild(texpr)
		}
	}

	return trx.Emit(fn)
}

// emitTemplate owns the declaration a template wraps: the inner node's own
// case sees template_declaration as its parent and walks through without
// emitting, so the vertex carries the template parameters exactly once
func emitTemplate(ctx context.Context, trx *parsers.TransformContext, node parsers.ParserNode) error {
	params, hasParams := node.ChildByFieldName("parameters")

	var inner parsers.ParserNode
	found := false

	node.IterateChildren(func(child parsers.ParserNode) (bool, error) {
		switch child.Kind {
		case "function_definition", "declaration", "field_declaration",
			"class_specifier", "struct_specifier", "union_specifier", "alias_declaration":
			inner = child
			found = true
			return false, nil
		}
		return true, nil
	})

	if !found {
		return trx.WalkChildren(ctx)
	}

	switch inner.Kind {
	case "function_definition":
		return emitFunction(ctx, trx, node, inner, params, hasParams)

	case "declaration", "field_declaration":
		if declNode, ok := inner.ChildByFieldName("declarator"); ok {
			if _, ok := findFunctionDeclarator(declNode); ok {
				return emitFunction(ctx, trx, node, inner, params, hasParams)
			}
		}

		return emitDeclaration(ctx, trx, inner)

	case "alias_declaration":
		return emitTemplateAlias(trx, node, inner, params, hasParams)

	default:
		return emitType(ctx, trx, node, inner, params, hasParams)
	}
}

func emitInclude(trx *parsers.TransformContext, node parsers.ParserNode) error {
	pathNode, ok := node.ChildByFieldName("path")

	if !ok || !hasSourceWidth(pathNode) {
		return nil
	}

	impExpr := ast.NewASTImportStatement(ast.NewASTNodeContainer(pathNode))

	switch pathNode.Kind {
	case "string_literal":
		if fragment, ok := pathNode.GetNthChildByKind("string_content", 0); ok && hasSourceWidth(fragment) {
			impExpr.Reference = ast.NewASTSymbol(ast.NewASTNodeContainer(pathNode), fragment.GetTextContent())
		}
	case "system_lib_string":
		impExpr.Reference = ast.NewASTSymbol(ast.NewASTNodeContainer(pathNode), strings.Trim(pathNode.GetTextContent(), "<>"))
	}

	if impExpr.Reference == nil {
		return nil
	}

	return trx.Emit(impExpr)
}

func ownedByTemplate(node parsers.ParserNode) bool {
	kind, _ := node.ParentKind()
	return kind == "template_declaration"
}

func CTransformer(ctx context.Context, trx *parsers.TransformContext, node parsers.ParserNode) error {
	switch node.Kind {
	case "preproc_include":
		return emitInclude(trx, node)

	case "preproc_def", "preproc_function_def":
		// A macro must leave a node on its name: macro invocations parse as
		// ordinary calls and their definitions resolve here
		nameNode, ok := node.ChildByFieldName("name")

		if !ok || !hasSourceWidth(nameNode) {
			return nil
		}

		decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(node))
		decl.Names = append(decl.Names, ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent()))

		return trx.Emit(decl)

	case "function_definition":
		if ownedByTemplate(node) {
			return trx.WalkChildren(ctx)
		}

		var zero parsers.ParserNode
		return emitFunction(ctx, trx, node, node, zero, false)

	case "declaration", "field_declaration":
		if ownedByTemplate(node) {
			return trx.WalkChildren(ctx)
		}

		return emitDeclaration(ctx, trx, node)

	case "type_definition":
		return emitTypedef(ctx, trx, node)

	case "struct_specifier", "union_specifier", "class_specifier", "enum_specifier":
		kind, _ := node.ParentKind()

		if kind == "type_definition" || kind == "template_declaration" {
			return trx.WalkChildren(ctx)
		}

		if _, ok := node.ChildByFieldName("body"); !ok {
			// A forward declaration statement leaves a node on its name the
			// way prototypes do — std:: types resolve into libc++'s forward
			// headers; a bodiless specifier elsewhere is a type reference
			// the enclosing annotation already mapped
			switch kind {
			case "translation_unit", "declaration_list", "field_declaration_list",
				"preproc_if", "preproc_ifdef", "preproc_else", "preproc_elif", "linkage_specification":
				if nameNode, ok := node.ChildByFieldName("name"); ok {
					if sym := declaredSymbol(nameNode); sym != nil {
						return trx.Emit(sym)
					}
				}
			}

			return trx.WalkChildren(ctx)
		}

		var zero parsers.ParserNode
		return emitType(ctx, trx, node, node, zero, false)

	case "enumerator":
		nameNode, ok := node.ChildByFieldName("name")

		if !ok || !hasSourceWidth(nameNode) {
			return nil
		}

		if _, ok := node.ChildByFieldName("value"); ok {
			decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(node))
			decl.Names = append(decl.Names, ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent()))
			return trx.Emit(decl)
		}

		// A bare enumerator spans exactly its name; a declaration node would
		// share the range and shadow the symbol
		return trx.Emit(ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent()))

	case "alias_declaration":
		if ownedByTemplate(node) {
			return nil
		}

		decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(node))

		if nameNode, ok := node.ChildByFieldName("name"); ok && hasSourceWidth(nameNode) {
			decl.Names = append(decl.Names, ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent()))
		}

		if typeNode, ok := node.ChildByFieldName("type"); ok {
			decl.Type = typeExpression(typeNode)
		}

		if len(decl.Names) == 0 && decl.Type == nil {
			return nil
		}

		return trx.Emit(decl)

	case "namespace_definition":
		fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))

		if nameNode, ok := node.ChildByFieldName("name"); ok {
			switch nameNode.Kind {
			case "namespace_identifier", "identifier":
				if hasSourceWidth(nameNode) {
					fn.Name = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
				}
			case "nested_namespace_specifier":
				// `namespace a::b` declares b here; the leading names reopen
				// enclosing scopes
				var last parsers.ParserNode
				found := false

				nameNode.IterateChildren(func(child parsers.ParserNode) (bool, error) {
					if child.Kind == "namespace_identifier" && hasSourceWidth(child) {
						last = child
						found = true
					}
					return true, nil
				})

				if found {
					fn.Name = ast.NewASTSymbol(ast.NewASTNodeContainer(last), last.GetTextContent())
				}
			}
		}

		if bodyNode, ok := node.ChildByFieldName("body"); ok {
			fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(bodyNode))
		}

		if err := trx.Emit(fn); err != nil {
			return err
		}

		return trx.WalkChildrenInto(ctx, fn)

	case "template_declaration":
		return emitTemplate(ctx, trx, node)

	case "lambda_expression":
		fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))

		if declNode, ok := node.ChildByFieldName("declarator"); ok {
			appendParameters(fn, declNode)

			if trailing, ok := declNode.GetNthChildByKind("trailing_return_type", 0); ok {
				fn.ReturnType = typeExpression(trailing)
			}
		}

		if bodyNode, ok := node.ChildByFieldName("body"); ok {
			fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(bodyNode))
		}

		if err := trx.Emit(fn); err != nil {
			return err
		}

		return trx.WalkChildrenInto(ctx, fn)

	case "call_expression":
		callExpr := ast.NewASTCallExpression(ast.NewASTNodeContainer(node))

		if fnNode, ok := node.ChildByFieldName("function"); ok {
			switch fnNode.Kind {
			case "identifier":
				if hasSourceWidth(fnNode) {
					callExpr.Symbol = ast.NewASTSymbol(ast.NewASTNodeContainer(fnNode), fnNode.GetTextContent())
				}

			case "field_expression":
				// Skips over chained calls (e.g. x.y().z()), chained calls
				// are handled below at WalkChildrenInto
				if argNode, ok := fnNode.ChildByFieldName("argument"); ok && argNode.Kind == "identifier" && hasSourceWidth(argNode) {
					callExpr.Namespace = ast.NewASTSymbol(ast.NewASTNodeContainer(argNode), argNode.GetTextContent())
				}

				if fieldNode, ok := fnNode.ChildByFieldName("field"); ok {
					callExpr.Symbol = declaredSymbol(fieldNode)
				}

			case "qualified_identifier", "template_function":
				callExpr.Symbol = declaredSymbol(fnNode)

				if scopeNode, ok := fnNode.ChildByFieldName("scope"); ok && scopeNode.Kind == "namespace_identifier" && hasSourceWidth(scopeNode) {
					callExpr.Namespace = ast.NewASTSymbol(ast.NewASTNodeContainer(scopeNode), scopeNode.GetTextContent())
				}
			}
		}

		// A cast keyword parses as a callee but has no definition site
		if callExpr.Symbol != nil {
			switch callExpr.Symbol.Name {
			case "static_cast", "dynamic_cast", "const_cast", "reinterpret_cast":
				callExpr.Symbol = nil
			}
		}

		// No identifiable callee (e.g. a call through a function pointer);
		// its children still index under the enclosing block
		if callExpr.Symbol == nil {
			return trx.WalkChildren(ctx)
		}

		appendCallArguments(callExpr, node)

		if err := trx.Emit(callExpr); err != nil {
			return err
		}
		// Handles chained operands
		return trx.WalkChildrenInto(ctx, callExpr)

	case "new_expression":
		callExpr := ast.NewASTCallExpression(ast.NewASTNodeContainer(node))

		if typeNode, ok := node.ChildByFieldName("type"); ok {
			callExpr.Symbol = declaredSymbol(typeNode)

			if scopeNode, ok := typeNode.ChildByFieldName("scope"); ok && scopeNode.Kind == "namespace_identifier" && hasSourceWidth(scopeNode) {
				callExpr.Namespace = ast.NewASTSymbol(ast.NewASTNodeContainer(scopeNode), scopeNode.GetTextContent())
			}
		}

		if callExpr.Symbol == nil {
			return trx.WalkChildren(ctx)
		}

		appendCallArguments(callExpr, node)

		if err := trx.Emit(callExpr); err != nil {
			return err
		}

		return trx.WalkChildrenInto(ctx, callExpr)

	case "for_range_loop":
		// A definition of the loop variable's use resolves onto its binding,
		// which needs a node; a bare symbol avoids a declaration shadowing it
		// at the same range
		if declNode, ok := node.ChildByFieldName("declarator"); ok {
			if nameNode, ok := declaredName(declNode); ok {
				syms := []*ast.ASTSymbol{}

				if nameNode.Kind == "structured_binding_declarator" {
					syms = collectIdentifiers(nameNode)
				} else {
					syms = append(syms, ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent()))
				}

				for _, sym := range syms {
					if err := trx.Emit(sym); err != nil {
						return err
					}
				}
			}
		}

		return trx.WalkChildren(ctx)

	case "return_statement":
		ret := ast.NewASTReturnStatement(ast.NewASTNodeContainer(node))

		// Capture plain identifier return values as symbol references
		node.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
			if pn.Kind == "identifier" {
				ret.AppendChild(ast.NewASTSymbol(ast.NewASTNodeContainer(pn), pn.GetTextContent()))
			}
			return true, nil
		})

		if err := trx.Emit(ret); err != nil {
			return err
		}

		return trx.WalkChildrenInto(ctx, ret)

	case "comment":
		return nil

	default:
		return trx.WalkChildren(ctx)
	}
}

func appendCallArguments(callExpr *ast.ASTCallExpression, node parsers.ParserNode) {
	argList, ok := node.ChildByFieldName("arguments")

	if !ok {
		return
	}

	argList.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
		syms := collectIdentifiers(pn)
		if len(syms) == 0 {
			callExpr.Arguments = append(callExpr.Arguments, ast.NewASTFuncArgument(ast.NewASTNodeContainer(pn), nil, nil))
		} else {
			for _, sym := range syms {
				callExpr.Arguments = append(callExpr.Arguments, ast.NewASTFuncArgument(sym.GetContainer(), sym, nil))
			}
		}
		return true, nil
	})
}

// CLanguageSupportDefinition covers the sibling dialects C and C++: one
// transformer and one server (clangd), with the grammar and the didOpen
// languageId varying per dialect. Headers (.h) parse with the C++ grammar,
// its superset, so the class declarations C++ headers hold stay parseable;
// clangd tells the languages apart from compile flags, not the didOpen id.
type CLanguageSupportDefinition struct {
	languageID string
	cpp        bool
}

func NewCLanguageSupportDefinition() *CLanguageSupportDefinition {
	return &CLanguageSupportDefinition{languageID: "c"}
}

func NewCPPLanguageSupportDefinition() *CLanguageSupportDefinition {
	clsd := NewCLanguageSupportDefinition()
	clsd.languageID = "cpp"
	clsd.cpp = true
	return clsd
}

var (
	// clangd resolves the standard library into the platform toolchain: SDK
	// and system headers under usr/include (which also covers libc++ and
	// libstdc++ under usr/include/c++), compiler builtins under lib/clang,
	// and the MSVC and Windows SDK roots on Windows
	systemIncludeRE = regexp.MustCompile(`(?i)/(usr/include|usr/lib/gcc|lib/clang/[^/]+/include)/`)
	windowsSDKRE    = regexp.MustCompile(`(?i)/(windows kits|msvc|microsoft visual studio)/`)
	packageRootRE   = regexp.MustCompile(`(?i)/(vcpkg[^/]*|conan[^/]*|homebrew|usr/local/include)/`)
)

func (clsd *CLanguageSupportDefinition) ClassifyImportType(s *common.Source) common.DependencyType {
	p := s.Path

	// Workspace-relative paths climb out with ../ before reaching the
	// toolchain; anchoring them back keeps the segments comparable
	if !filepath.IsAbs(p) && s.Workspace != "" {
		p = filepath.Join(s.Workspace, p)
	}

	p = filepath.ToSlash(filepath.Clean(p))

	if systemIncludeRE.MatchString(p) || windowsSDKRE.MatchString(p) {
		return common.StandardLibraryDependency
	}

	if packageRootRE.MatchString(p) {
		return common.PackageDependency
	}

	return common.LocalDependency
}

func (clsd *CLanguageSupportDefinition) GetLSPServerRequirement() lsp.ServerRequirement {
	// banstructlit:ignore
	return lsp.ServerRequirement{
		Name:           clangdName,
		InstallCommand: clangdInstallCommand(),
		Locate:         clangdPath,
	}
}

func (clsd *CLanguageSupportDefinition) NewLSPServer(ctx context.Context) (*lsp.ServerProcess, error) {
	execPath, err := clangdPath(ctx)

	if err != nil {
		return nil, err
	}

	// clangd speaks LSP over stdio by default; its logging goes to stderr
	// and must stay off stdout to keep Content-Length framing intact
	cmd := exec.CommandContext(ctx, execPath, "--log=error")
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return lsp.NewServerProcess(stdout, stdin, cmd.Wait, func() error {
		return cmd.Process.Kill()
	}), nil
}

func (clsd *CLanguageSupportDefinition) Parse(ctx context.Context, src *common.Source, tree *tree_sitter.Tree) (*ast.ASTModule, error) {
	name := strings.TrimSuffix(filepath.Base(src.Path), filepath.Ext(src.Path))

	root := ast.NewASTModule(ast.NewASTNodeContainer(
		parsers.NewParserNode(src, tree.RootNode()),
	), name)

	if err := parsers.WalkTransformTree(ctx, src, tree, root, CTransformer); err != nil {
		return nil, err
	}

	return root, nil
}

func (clsd *CLanguageSupportDefinition) GetLanguageID() string {
	return clsd.languageID
}

func (clsd *CLanguageSupportDefinition) GetTreeSitterLanguage() *tree_sitter.Language {
	if clsd.cpp {
		return tree_sitter.NewLanguage(tree_sitter_cpp.Language())
	}

	return tree_sitter.NewLanguage(tree_sitter_c.Language())
}
