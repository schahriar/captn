package languages_typescript

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
	tree_sitter_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// hasSourceWidth reports whether a node spans any bytes; tree-sitter error
// recovery produces zero-width nodes that must never become AST nodes (the
// interval index rejects empty ranges)
func hasSourceWidth(node parsers.ParserNode) bool {
	return node.Range.StartByte < node.Range.EndByte
}

func isNameKind(kind string) bool {
	switch kind {
	case "identifier", "property_identifier", "private_property_identifier", "type_identifier":
		return true
	}
	return false
}

func nameSymbol(node parsers.ParserNode) *ast.ASTSymbol {
	nameNode, ok := node.ChildByFieldName("name")

	if !ok || !isNameKind(nameNode.Kind) || !hasSourceWidth(nameNode) {
		return nil
	}

	return ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
}

func collectIdentifiers(node parsers.ParserNode) []*ast.ASTSymbol {
	if node.Kind == "identifier" {
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

// collectBindingNames gathers the identifiers a binding pattern declares. The
// right-hand side of a default value is an expression, not a binding, so
// assignment patterns recurse into their left side only.
func collectBindingNames(node parsers.ParserNode) []parsers.ParserNode {
	switch node.Kind {
	case "identifier", "shorthand_property_identifier_pattern":
		if !hasSourceWidth(node) {
			return nil
		}
		return []parsers.ParserNode{node}

	case "pair_pattern":
		if valueNode, ok := node.ChildByFieldName("value"); ok {
			return collectBindingNames(valueNode)
		}
		return nil

	case "assignment_pattern", "object_assignment_pattern":
		if leftNode, ok := node.ChildByFieldName("left"); ok {
			return collectBindingNames(leftNode)
		}
		return nil

	case "object_pattern", "array_pattern", "rest_pattern":
		var names []parsers.ParserNode
		node.IterateChildren(func(child parsers.ParserNode) (bool, error) {
			names = append(names, collectBindingNames(child)...)
			return true, nil
		})
		return names
	}

	return nil
}

// typeExpression maps a type onto the identifier a definition resolves to.
// Predefined types (string, number, void, ...) are keywords with no
// definition site and yield nothing. Nameless composites (unions, arrays,
// object types, function types) fold flat: the first named type inside is
// the head and the rest are its arguments.
func typeExpression(node parsers.ParserNode) *ast.ASTTypeExpression {
	switch node.Kind {
	case "type_identifier":
		if !hasSourceWidth(node) {
			return nil
		}

		return ast.NewASTTypeExpression(ast.NewASTNodeContainer(node), node.GetTextContent())

	case "nested_type_identifier":
		nameNode, ok := node.ChildByFieldName("name")

		if !ok || !hasSourceWidth(nameNode) {
			return nil
		}

		texpr := ast.NewASTTypeExpression(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())

		if moduleNode, ok := node.ChildByFieldName("module"); ok && hasSourceWidth(moduleNode) {
			texpr.Namespace = ast.NewASTSymbol(ast.NewASTNodeContainer(moduleNode), moduleNode.GetTextContent())
		}

		return texpr

	case "generic_type":
		headNode, ok := node.ChildByFieldName("name")

		if !ok {
			return nil
		}

		head := typeExpression(headNode)

		if head == nil {
			return nil
		}

		if argsNode, ok := node.ChildByFieldName("type_arguments"); ok {
			head.Arguments = append(head.Arguments, collectTypes(argsNode)...)
		}

		return head
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
		case "type_identifier", "nested_type_identifier", "generic_type":
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

func annotatedType(node parsers.ParserNode) *ast.ASTTypeExpression {
	typeNode, ok := node.ChildByFieldName("type")

	if !ok {
		return nil
	}

	return typeExpression(typeNode)
}

func returnType(node parsers.ParserNode) *ast.ASTTypeExpression {
	resultNode, ok := node.ChildByFieldName("return_type")

	if !ok {
		return nil
	}

	return typeExpression(resultNode)
}

// A destructuring pattern binds several names in one parameter; each name past
// the first gets an argument on its own identifier so a definition resolving
// onto it still finds a node there
func appendParameter(fn *ast.ASTFuncExpression, decl parsers.ParserNode) {
	texpr := annotatedType(decl)

	nameNode, ok := decl.ChildByFieldName("name")

	if !ok {
		nameNode, ok = decl.ChildByFieldName("pattern")
	}

	var names []parsers.ParserNode

	if ok {
		names = collectBindingNames(nameNode)
	}

	if len(names) == 0 {
		fn.Arguments = append(fn.Arguments, ast.NewASTFuncArgument(ast.NewASTNodeContainer(decl), nil, texpr))
		return
	}

	for i, name := range names {
		sym := ast.NewASTSymbol(ast.NewASTNodeContainer(name), name.GetTextContent())

		if i == 0 {
			fn.Arguments = append(fn.Arguments, ast.NewASTFuncArgument(ast.NewASTNodeContainer(decl), sym, texpr))
			continue
		}

		fn.Arguments = append(fn.Arguments, ast.NewASTFuncArgument(ast.NewASTNodeContainer(name), sym, nil))
	}
}

func appendParameters(fn *ast.ASTFuncExpression, decl parsers.ParserNode) {
	list, ok := decl.ChildByFieldName("parameters")

	if !ok {
		// An arrow function can take a single bare identifier
		if paramNode, ok := decl.ChildByFieldName("parameter"); ok && hasSourceWidth(paramNode) {
			sym := ast.NewASTSymbol(ast.NewASTNodeContainer(paramNode), paramNode.GetTextContent())
			fn.Arguments = append(fn.Arguments, ast.NewASTFuncArgument(ast.NewASTNodeContainer(paramNode), sym, nil))
		}
		return
	}

	list.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
		switch pn.Kind {
		case "required_parameter", "optional_parameter":
			appendParameter(fn, pn)
		}
		return true, nil
	})
}

func appendTypeParameters(fn *ast.ASTFuncExpression, decl parsers.ParserNode) {
	list, ok := decl.ChildByFieldName("type_parameters")

	if !ok {
		return
	}

	list.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
		if pn.Kind != "type_parameter" {
			return true, nil
		}

		var sym *ast.ASTSymbol

		if nameNode, ok := pn.ChildByFieldName("name"); ok && hasSourceWidth(nameNode) {
			sym = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
		}

		var texpr *ast.ASTTypeExpression

		if constraintNode, ok := pn.ChildByFieldName("constraint"); ok {
			texpr = typeExpression(constraintNode)
		}

		fn.Arguments = append(fn.Arguments, ast.NewASTFuncArgument(ast.NewASTNodeContainer(pn), sym, texpr))

		return true, nil
	})
}

func emitFunction(ctx context.Context, trx *parsers.TransformContext, node parsers.ParserNode) error {
	fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))
	fn.Name = nameSymbol(node)

	appendTypeParameters(fn, node)
	appendParameters(fn, node)
	fn.ReturnType = returnType(node)

	// A signature has no body to narrow to; its block keeps the shadow
	// deliberately so a snippet on that line roots at the enclosing type
	if bodyNode, ok := node.ChildByFieldName("body"); ok {
		fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(bodyNode))
	}

	if err := trx.Emit(fn); err != nil {
		return err
	}

	return trx.WalkChildrenInto(ctx, fn)
}

// The extends value of a class is an expression, resolvable as a type only
// when it names one directly; mixin calls and other expressions are skipped
func heritageType(node parsers.ParserNode) *ast.ASTTypeExpression {
	switch node.Kind {
	case "identifier":
		if !hasSourceWidth(node) {
			return nil
		}

		return ast.NewASTTypeExpression(ast.NewASTNodeContainer(node), node.GetTextContent())

	case "member_expression":
		propNode, ok := node.ChildByFieldName("property")

		if !ok || !hasSourceWidth(propNode) {
			return nil
		}

		texpr := ast.NewASTTypeExpression(ast.NewASTNodeContainer(propNode), propNode.GetTextContent())

		if objNode, ok := node.ChildByFieldName("object"); ok && hasSourceWidth(objNode) {
			texpr.Namespace = ast.NewASTSymbol(ast.NewASTNodeContainer(objNode), objNode.GetTextContent())
		}

		return texpr
	}

	return nil
}

func appendHeritage(fn *ast.ASTFuncExpression, node parsers.ParserNode) {
	heritage, ok := node.GetNthChildByKind("class_heritage", 0)

	if !ok {
		return
	}

	heritage.IterateChildren(func(clause parsers.ParserNode) (bool, error) {
		switch clause.Kind {
		case "extends_clause":
			valueNode, ok := clause.ChildByFieldName("value")

			if !ok {
				return true, nil
			}

			texpr := heritageType(valueNode)

			if texpr == nil {
				return true, nil
			}

			if argsNode, ok := clause.ChildByFieldName("type_arguments"); ok {
				texpr.Arguments = append(texpr.Arguments, collectTypes(argsNode)...)
			}

			fn.AppendChild(texpr)

		case "implements_clause":
			for _, texpr := range collectTypes(clause) {
				fn.AppendChild(texpr)
			}
		}

		return true, nil
	})
}

func emitClass(ctx context.Context, trx *parsers.TransformContext, node parsers.ParserNode) error {
	fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))
	fn.Name = nameSymbol(node)

	appendTypeParameters(fn, node)

	if bodyNode, ok := node.ChildByFieldName("body"); ok {
		fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(bodyNode))
	}

	appendHeritage(fn, node)

	if err := trx.Emit(fn); err != nil {
		return err
	}

	return trx.WalkChildrenInto(ctx, fn)
}

func emitInterface(trx *parsers.TransformContext, node parsers.ParserNode) error {
	fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))
	fn.Name = nameSymbol(node)

	appendTypeParameters(fn, node)

	bodyNode, hasBody := node.ChildByFieldName("body")

	if hasBody {
		fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(bodyNode))
	}

	if extendsNode, ok := node.GetNthChildByKind("extends_type_clause", 0); ok {
		for _, texpr := range collectTypes(extendsNode) {
			fn.AppendChild(texpr)
		}
	}

	if hasBody {
		appendTypeMembers(fn, bodyNode)
	}

	return trx.Emit(fn)
}

// appendTypeMembers builds the member subtree of an interface_body or
// object_type explicitly, the way the golang reference builds struct fields
// and interface methods
func appendTypeMembers(fn *ast.ASTFuncExpression, bodyNode parsers.ParserNode) {
	bodyNode.IterateChildren(func(member parsers.ParserNode) (bool, error) {
		switch member.Kind {
		case "property_signature":
			decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(member))

			if sym := nameSymbol(member); sym != nil {
				decl.Names = append(decl.Names, sym)
			}

			decl.Type = annotatedType(member)

			if len(decl.Names) > 0 || decl.Type != nil {
				fn.AppendChild(decl)
			}

		// Call and construct signatures stay anonymous; definitions of a
		// callable interface land on them, and the narrowed reply needs a
		// parameter or type parameter node to resolve onto
		case "method_signature", "call_signature", "construct_signature":
			method := ast.NewASTFuncExpression(ast.NewASTNodeContainer(member))
			method.Name = nameSymbol(member)

			appendTypeParameters(method, member)
			appendParameters(method, member)
			method.ReturnType = returnType(member)

			// A construct signature keeps its return under field `type`
			if method.ReturnType == nil && member.Kind == "construct_signature" {
				method.ReturnType = annotatedType(member)
			}

			fn.AppendChild(method)
		}

		return true, nil
	})
}

func emitEnum(ctx context.Context, trx *parsers.TransformContext, node parsers.ParserNode) error {
	fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))
	fn.Name = nameSymbol(node)

	bodyNode, ok := node.ChildByFieldName("body")

	if !ok {
		return trx.Emit(fn)
	}

	fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(bodyNode))

	bodyNode.IterateChildren(func(member parsers.ParserNode) (bool, error) {
		switch member.Kind {
		// A bare member is only a name; a declaration node would share its
		// exact range and shadow it in the interval index
		case "property_identifier":
			if hasSourceWidth(member) {
				fn.AppendChild(ast.NewASTSymbol(ast.NewASTNodeContainer(member), member.GetTextContent()))
			}

		case "enum_assignment":
			decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(member))

			if sym := nameSymbol(member); sym != nil {
				decl.Names = append(decl.Names, sym)
			}

			if len(decl.Names) > 0 {
				fn.AppendChild(decl)
			}
		}

		return true, nil
	})

	if err := trx.Emit(fn); err != nil {
		return err
	}

	// Calls inside member initializers still land in the enum's block; the
	// member kinds carry no cases, so the walk cannot re-emit them
	return trx.WalkChildrenInto(ctx, fn)
}

// A member-carrying or generic alias is a type whose members and parameters
// need definition-site nodes, so it maps like an interface; a plain alias is
// a declaration and stays off the vertex set
func emitTypeAlias(trx *parsers.TransformContext, node parsers.ParserNode) error {
	valueNode, hasValue := node.ChildByFieldName("value")
	_, generic := node.ChildByFieldName("type_parameters")

	if hasValue && !generic && valueNode.Kind != "object_type" {
		decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(node))

		if sym := nameSymbol(node); sym != nil {
			decl.Names = append(decl.Names, sym)
		}

		decl.Type = typeExpression(valueNode)

		return trx.Emit(decl)
	}

	fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))
	fn.Name = nameSymbol(node)

	appendTypeParameters(fn, node)

	if !hasValue {
		return trx.Emit(fn)
	}

	fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(valueNode))

	if valueNode.Kind == "object_type" {
		appendTypeMembers(fn, valueNode)
	} else if texpr := typeExpression(valueNode); texpr != nil {
		fn.AppendChild(texpr)
	}

	return trx.Emit(fn)
}

func emitImport(trx *parsers.TransformContext, node parsers.ParserNode) error {
	sourceNode, ok := node.ChildByFieldName("source")

	// `import x = require("y")` keeps its source inside the require clause
	if !ok {
		if clause, cok := node.GetNthChildByKind("import_require_clause", 0); cok {
			sourceNode, ok = clause.ChildByFieldName("source")
		}
	}

	if !ok || !hasSourceWidth(sourceNode) {
		return nil
	}

	impExpr := ast.NewASTImportStatement(ast.NewASTNodeContainer(sourceNode))

	if fragment, ok := sourceNode.GetNthChildByKind("string_fragment", 0); ok && hasSourceWidth(fragment) {
		impExpr.Reference = ast.NewASTSymbol(ast.NewASTNodeContainer(sourceNode), fragment.GetTextContent())
	}

	if clause, ok := node.GetNthChildByKind("import_clause", 0); ok {
		if nsNode, ok := clause.GetNthChildByKind("namespace_import", 0); ok {
			if ident, ok := nsNode.GetNthChildByKind("identifier", 0); ok && hasSourceWidth(ident) {
				impExpr.Namespace = ast.NewASTSymbol(ast.NewASTNodeContainer(ident), ident.GetTextContent())
			}
		} else if ident, ok := clause.GetNthChildByKind("identifier", 0); ok && hasSourceWidth(ident) {
			impExpr.Namespace = ast.NewASTSymbol(ast.NewASTNodeContainer(ident), ident.GetTextContent())
		}
	} else if clause, ok := node.GetNthChildByKind("import_require_clause", 0); ok {
		if ident, ok := clause.GetNthChildByKind("identifier", 0); ok && hasSourceWidth(ident) {
			impExpr.Namespace = ast.NewASTSymbol(ast.NewASTNodeContainer(ident), ident.GetTextContent())
		}
	}

	if impExpr.Reference == nil {
		return nil
	}

	if err := trx.Emit(impExpr); err != nil {
		return err
	}

	// Named import bindings get loose symbols of their own: until the
	// configured project loads, tsserver answers a definition with the local
	// binding instead of the imported declaration, and that reply must still
	// resolve onto exactly one node
	var bindings []parsers.ParserNode

	if clause, ok := node.GetNthChildByKind("import_clause", 0); ok {
		if named, ok := clause.GetNthChildByKind("named_imports", 0); ok {
			named.IterateChildren(func(spec parsers.ParserNode) (bool, error) {
				if spec.Kind != "import_specifier" {
					return true, nil
				}

				bound, ok := spec.ChildByFieldName("alias")

				if !ok {
					bound, ok = spec.ChildByFieldName("name")
				}

				if ok && bound.Kind == "identifier" && hasSourceWidth(bound) {
					bindings = append(bindings, bound)
				}

				return true, nil
			})
		}
	}

	for _, bound := range bindings {
		if err := trx.Emit(ast.NewASTSymbol(ast.NewASTNodeContainer(bound), bound.GetTextContent())); err != nil {
			return err
		}
	}

	return nil
}

func TypescriptTransformer(ctx context.Context, trx *parsers.TransformContext, node parsers.ParserNode) error {
	switch node.Kind {
	case "import_statement":
		return emitImport(trx, node)

	case "export_statement":
		// Only a re-export (`export ... from "x"`) resolves a module; plain
		// exports carry declarations that the walk maps on its own
		if _, ok := node.ChildByFieldName("source"); ok {
			return emitImport(trx, node)
		}

		return trx.WalkChildren(ctx)

	case "function_declaration", "generator_function_declaration", "function_expression", "generator_function",
		"function_signature", "method_definition", "method_signature", "abstract_method_signature":
		return emitFunction(ctx, trx, node)

	case "arrow_function":
		fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))

		appendTypeParameters(fn, node)
		appendParameters(fn, node)
		fn.ReturnType = returnType(node)

		if bodyNode, ok := node.ChildByFieldName("body"); ok {
			fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(bodyNode))
		}

		if err := trx.Emit(fn); err != nil {
			return err
		}

		return trx.WalkChildrenInto(ctx, fn)

	case "class_declaration", "abstract_class_declaration":
		return emitClass(ctx, trx, node)

	case "class":
		// The `class` keyword token shares this kind; only the expression
		// form carries a body
		if _, ok := node.ChildByFieldName("body"); !ok {
			return trx.WalkChildren(ctx)
		}

		return emitClass(ctx, trx, node)

	case "interface_declaration":
		return emitInterface(trx, node)

	case "enum_declaration":
		return emitEnum(ctx, trx, node)

	case "type_alias_declaration":
		return emitTypeAlias(trx, node)

	case "internal_module", "module":
		// The `module` keyword token shares this kind; only the declaration
		// form carries a name
		if _, ok := node.ChildByFieldName("name"); !ok {
			return trx.WalkChildren(ctx)
		}

		fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))
		fn.Name = nameSymbol(node)

		if bodyNode, ok := node.ChildByFieldName("body"); ok {
			fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(bodyNode))
		}

		if err := trx.Emit(fn); err != nil {
			return err
		}

		return trx.WalkChildrenInto(ctx, fn)

	case "variable_declarator":
		decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(node))

		if nameNode, ok := node.ChildByFieldName("name"); ok {
			for _, name := range collectBindingNames(nameNode) {
				decl.Names = append(decl.Names, ast.NewASTSymbol(ast.NewASTNodeContainer(name), name.GetTextContent()))
			}
		}

		decl.Type = annotatedType(node)

		if err := trx.Emit(decl); err != nil {
			return err
		}

		// RHS: walk into decl so calls and lambdas on the right-hand side
		// land in Virtual
		return trx.WalkChildrenInto(ctx, decl)

	case "public_field_definition":
		decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(node))

		if sym := nameSymbol(node); sym != nil {
			decl.Names = append(decl.Names, sym)
		}

		decl.Type = annotatedType(node)

		if err := trx.Emit(decl); err != nil {
			return err
		}

		return trx.WalkChildrenInto(ctx, decl)

	case "call_expression", "new_expression":
		callExpr := ast.NewASTCallExpression(ast.NewASTNodeContainer(node))

		fnNode, ok := node.ChildByFieldName("function")

		if !ok {
			fnNode, ok = node.ChildByFieldName("constructor")
		}

		if ok {
			switch fnNode.Kind {
			case "member_expression":
				// Skips over chained calls (e.g. x.y().z()), chained calls are handled below at WalkChildrenInto
				if objNode, ok := fnNode.ChildByFieldName("object"); ok {
					if objNode.Kind == "identifier" && hasSourceWidth(objNode) {
						callExpr.Namespace = ast.NewASTSymbol(ast.NewASTNodeContainer(objNode), objNode.GetTextContent())
					}
				}
				if propNode, ok := fnNode.ChildByFieldName("property"); ok && hasSourceWidth(propNode) {
					callExpr.Symbol = ast.NewASTSymbol(ast.NewASTNodeContainer(propNode), propNode.GetTextContent())
				}
			case "identifier":
				if hasSourceWidth(fnNode) {
					callExpr.Symbol = ast.NewASTSymbol(ast.NewASTNodeContainer(fnNode), fnNode.GetTextContent())
				}
			}
		}

		// No identifiable callee (e.g. an IIFE or dynamic import); its
		// children still index under the enclosing block
		if callExpr.Symbol == nil {
			return trx.WalkChildren(ctx)
		}

		if argList, ok := node.ChildByFieldName("arguments"); ok {
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

		// A CommonJS require is how the JavaScript dialect imports; without
		// this the dependency listing of a .cjs file is empty
		if node.Kind == "call_expression" && callExpr.Namespace == nil && callExpr.Symbol.Name == "require" {
			if argList, ok := node.ChildByFieldName("arguments"); ok {
				if pathNode, ok := argList.GetNthChildByKind("string", 0); ok && hasSourceWidth(pathNode) {
					impExpr := ast.NewASTImportStatement(ast.NewASTNodeContainer(pathNode))

					if fragment, ok := pathNode.GetNthChildByKind("string_fragment", 0); ok && hasSourceWidth(fragment) {
						impExpr.Reference = ast.NewASTSymbol(ast.NewASTNodeContainer(pathNode), fragment.GetTextContent())
					}

					if impExpr.Reference != nil {
						if err := trx.Emit(impExpr); err != nil {
							return err
						}
					}
				}
			}
		}

		if err := trx.Emit(callExpr); err != nil {
			return err
		}
		// Handles chained operands
		return trx.WalkChildrenInto(ctx, callExpr)

	case "for_in_statement":
		// A definition of a loop variable's use resolves onto its binding,
		// which needs a node; a bare identifier gets a symbol alone so no
		// declaration shadows it at the same range
		leftNode, ok := node.ChildByFieldName("left")

		if !ok {
			return trx.WalkChildren(ctx)
		}

		if leftNode.Kind == "identifier" {
			if hasSourceWidth(leftNode) {
				if err := trx.Emit(ast.NewASTSymbol(ast.NewASTNodeContainer(leftNode), leftNode.GetTextContent())); err != nil {
					return err
				}
			}

			return trx.WalkChildren(ctx)
		}

		names := collectBindingNames(leftNode)

		if len(names) == 0 {
			return trx.WalkChildren(ctx)
		}

		decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(leftNode))

		for _, name := range names {
			decl.Names = append(decl.Names, ast.NewASTSymbol(ast.NewASTNodeContainer(name), name.GetTextContent()))
		}

		if err := trx.Emit(decl); err != nil {
			return err
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

	case "type_annotation", "asserts_annotation", "type_predicate_annotation",
		"type_arguments", "type_parameters", "object_type":
		// Types are pulled explicitly where they annotate a node; walking
		// into them would re-emit what the annotation already carries.
		// object_type is otherwise reachable through as/satisfies casts,
		// where its member signatures would leak stray function vertices.
		return nil

	case "class_heritage":
		// The heritage types were appended by emitClass; walking on still
		// reaches a mixin call (`extends Base(Widget)`), which has no other
		// path into the graph. The identifier and type kinds inside carry no
		// cases, so nothing is emitted twice.
		return trx.WalkChildren(ctx)

	case "comment":
		return nil

	default:
		return trx.WalkChildren(ctx)
	}
}

// TypescriptLanguageSupportDefinition covers the sibling dialects TypeScript,
// TSX, JavaScript and JSX: one transformer and one server, with the grammar
// and the didOpen languageId varying per dialect. JavaScript parses with the
// TSX grammar, which is its superset and keeps JSX in .js files parseable.
type TypescriptLanguageSupportDefinition struct {
	languageID string
	tsx        bool
}

func NewTypescriptLanguageSupportDefinition() *TypescriptLanguageSupportDefinition {
	return &TypescriptLanguageSupportDefinition{languageID: "typescript"}
}

func NewTSXLanguageSupportDefinition() *TypescriptLanguageSupportDefinition {
	tlsd := NewTypescriptLanguageSupportDefinition()
	tlsd.languageID = "typescriptreact"
	tlsd.tsx = true
	return tlsd
}

func NewJavascriptLanguageSupportDefinition() *TypescriptLanguageSupportDefinition {
	tlsd := NewTypescriptLanguageSupportDefinition()
	tlsd.languageID = "javascript"
	tlsd.tsx = true
	return tlsd
}

func NewJSXLanguageSupportDefinition() *TypescriptLanguageSupportDefinition {
	tlsd := NewTypescriptLanguageSupportDefinition()
	tlsd.languageID = "javascriptreact"
	tlsd.tsx = true
	return tlsd
}

var (
	// tsserver resolves every ECMAScript builtin into the lib files of the
	// typescript package it runs with, and Node builtins into @types/node.
	// Anchored on a segment boundary: node_modules can sit at the very start
	// of a workspace-relative path.
	tsLibRE       = regexp.MustCompile(`(?i)(^|/)node_modules/typescript/lib/`)
	typesNodeRE   = regexp.MustCompile(`(?i)(^|/)node_modules/@types/node/`)
	nodeModulesRE = regexp.MustCompile(`(?i)(^|/)node_modules/`)
)

func (tlsd *TypescriptLanguageSupportDefinition) ClassifyImportType(s *common.Source) common.DependencyType {
	p := s.Path

	// Workspace-relative paths climb out with ../ before reaching a global
	// install; anchoring them back keeps the segments comparable
	if !filepath.IsAbs(p) && s.Workspace != "" {
		p = filepath.Join(s.Workspace, p)
	}

	p = filepath.ToSlash(filepath.Clean(p))

	if tsLibRE.MatchString(p) || typesNodeRE.MatchString(p) {
		return common.StandardLibraryDependency
	}

	if nodeModulesRE.MatchString(p) {
		return common.PackageDependency
	}

	return common.LocalDependency
}

func (tlsd *TypescriptLanguageSupportDefinition) GetLSPServerRequirement() lsp.ServerRequirement {
	// banstructlit:ignore
	return lsp.ServerRequirement{
		Name:           tsserverName,
		InstallCommand: tsserverInstallCommand,
		Locate:         tsserverPath,
	}
}

func (tlsd *TypescriptLanguageSupportDefinition) NewLSPServer(ctx context.Context) (*lsp.ServerProcess, error) {
	execPath, err := tsserverPath(ctx)

	if err != nil {
		return nil, err
	}

	// typescript-language-server only speaks LSP over stdio when asked;
	// without the flag it hangs at initialize
	cmd := exec.CommandContext(ctx, execPath, "--stdio")
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

func (tlsd *TypescriptLanguageSupportDefinition) Parse(ctx context.Context, src *common.Source, tree *tree_sitter.Tree) (*ast.ASTModule, error) {
	name := strings.TrimSuffix(filepath.Base(src.Path), filepath.Ext(src.Path))

	root := ast.NewASTModule(ast.NewASTNodeContainer(
		parsers.NewParserNode(src, tree.RootNode()),
	), name)

	if err := parsers.WalkTransformTree(ctx, src, tree, root, TypescriptTransformer); err != nil {
		return nil, err
	}

	return root, nil
}

func (tlsd *TypescriptLanguageSupportDefinition) GetLanguageID() string {
	return tlsd.languageID
}

func (tlsd *TypescriptLanguageSupportDefinition) GetTreeSitterLanguage() *tree_sitter.Language {
	if tlsd.tsx {
		return tree_sitter.NewLanguage(tree_sitter_typescript.LanguageTSX())
	}

	return tree_sitter.NewLanguage(tree_sitter_typescript.LanguageTypescript())
}
