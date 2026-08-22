package languages_rust

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/parsers"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_rust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
)

// hasSourceWidth reports whether a node spans any bytes; tree-sitter error
// recovery produces zero-width nodes that must never become AST nodes (the
// interval index rejects empty ranges)
func hasSourceWidth(node parsers.ParserNode) bool {
	return node.Range.StartByte < node.Range.EndByte
}

// typeExpression maps a type onto the identifier a definition resolves to.
// Nameless composites (references, tuples, slices, function types) fold flat:
// the first named type inside is the head and the rest are its arguments.
// Primitive types (u32, str, bool) have no definition site and yield nil.
func typeExpression(node parsers.ParserNode) *ast.ASTTypeExpression {
	switch node.Kind {
	case "type_identifier":
		if !hasSourceWidth(node) {
			return nil
		}

		return ast.NewASTTypeExpression(ast.NewASTNodeContainer(node), node.GetTextContent())

	case "scoped_type_identifier":
		nameNode, ok := node.ChildByFieldName("name")

		if !ok || !hasSourceWidth(nameNode) {
			return nil
		}

		texpr := ast.NewASTTypeExpression(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())

		if pathNode, ok := node.ChildByFieldName("path"); ok && pathNode.Kind == "identifier" && hasSourceWidth(pathNode) {
			texpr.Namespace = ast.NewASTSymbol(ast.NewASTNodeContainer(pathNode), pathNode.GetTextContent())
		}

		return texpr

	case "generic_type":
		headNode, ok := node.ChildByFieldName("type")

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

	return foldTypes(collectTypes(node))
}

func foldTypes(types []*ast.ASTTypeExpression) *ast.ASTTypeExpression {
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
		case "type_identifier", "scoped_type_identifier", "generic_type":
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

// collectPatternNames collects the identifiers a pattern binds. The type field
// of a struct or tuple-struct pattern is the variant being matched, not a
// binding, so it is skipped.
func collectPatternNames(node parsers.ParserNode) []*ast.ASTSymbol {
	if node.Kind == "identifier" || node.Kind == "shorthand_field_identifier" {
		if !hasSourceWidth(node) {
			return nil
		}
		return []*ast.ASTSymbol{ast.NewASTSymbol(ast.NewASTNodeContainer(node), node.GetTextContent())}
	}

	var syms []*ast.ASTSymbol

	node.IterateAllChildren(func(child parsers.ParserNode, field string) (bool, error) {
		if field == "type" {
			return true, nil
		}
		syms = append(syms, collectPatternNames(child)...)
		return true, nil
	})

	return syms
}

// emitBindingDeclaration emits an ASTDeclaration on a binding pattern (loop
// variable, if-let/while-let binding, match-arm binding) so LSP-resolved
// definition ranges always contain a symbol
func emitBindingDeclaration(trx *parsers.TransformContext, target parsers.ParserNode) error {
	names := collectPatternNames(target)

	if len(names) == 0 {
		return nil
	}

	decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(target))
	decl.Names = names

	return trx.Emit(decl)
}

// Several names can bind in one parameter pattern (`(a, b): (i32, i32)`); each
// name past the first gets an argument on its own identifier so a definition
// resolving onto it still finds a node there
func appendParameter(fn *ast.ASTFuncExpression, param parsers.ParserNode) {
	var texpr *ast.ASTTypeExpression

	if typeNode, ok := param.ChildByFieldName("type"); ok {
		texpr = typeExpression(typeNode)
	}

	var names []*ast.ASTSymbol

	if patternNode, ok := param.ChildByFieldName("pattern"); ok {
		names = collectPatternNames(patternNode)
	}

	if len(names) == 0 {
		fn.Arguments = append(fn.Arguments, ast.NewASTFuncArgument(ast.NewASTNodeContainer(param), nil, texpr))
		return
	}

	for i, sym := range names {
		if i == 0 {
			fn.Arguments = append(fn.Arguments, ast.NewASTFuncArgument(ast.NewASTNodeContainer(param), sym, texpr))
			continue
		}

		fn.Arguments = append(fn.Arguments, ast.NewASTFuncArgument(sym.GetContainer(), sym, nil))
	}
}

func appendParameters(fn *ast.ASTFuncExpression, list parsers.ParserNode) {
	list.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
		switch pn.Kind {
		case "parameter", "variadic_parameter":
			appendParameter(fn, pn)
		case "identifier":
			// Closure shorthand parameters carry no type annotation
			if hasSourceWidth(pn) {
				sym := ast.NewASTSymbol(ast.NewASTNodeContainer(pn), pn.GetTextContent())
				fn.Arguments = append(fn.Arguments, ast.NewASTFuncArgument(ast.NewASTNodeContainer(pn), sym, nil))
			}
		}
		return true, nil
	})
}

// Type parameters map to arguments of the declaring item: identifier on the
// parameter name, type on the first bound
func appendTypeParameters(fn *ast.ASTFuncExpression, decl parsers.ParserNode) {
	list, ok := decl.ChildByFieldName("type_parameters")

	if !ok {
		return
	}

	list.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
		switch pn.Kind {
		case "type_parameter", "const_parameter":
			nameNode, ok := pn.ChildByFieldName("name")

			if !ok || !hasSourceWidth(nameNode) {
				return true, nil
			}

			sym := ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())

			var texpr *ast.ASTTypeExpression

			if boundsNode, ok := pn.ChildByFieldName("bounds"); ok {
				texpr = foldTypes(collectTypes(boundsNode))
			} else if typeNode, ok := pn.ChildByFieldName("type"); ok {
				texpr = typeExpression(typeNode)
			}

			fn.Arguments = append(fn.Arguments, ast.NewASTFuncArgument(ast.NewASTNodeContainer(pn), sym, texpr))
		}
		return true, nil
	})
}

func typeParameterNames(decl parsers.ParserNode) []*ast.ASTSymbol {
	list, ok := decl.ChildByFieldName("type_parameters")

	if !ok {
		return nil
	}

	var names []*ast.ASTSymbol

	list.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
		switch pn.Kind {
		case "type_parameter", "const_parameter":
			if nameNode, ok := pn.ChildByFieldName("name"); ok && hasSourceWidth(nameNode) {
				names = append(names, ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent()))
			}
		}
		return true, nil
	})

	return names
}

func returnType(decl parsers.ParserNode) *ast.ASTTypeExpression {
	resultNode, ok := decl.ChildByFieldName("return_type")

	if !ok {
		return nil
	}

	return typeExpression(resultNode)
}

func appendStructFields(fn *ast.ASTFuncExpression, list parsers.ParserNode) {
	list.IterateChildren(func(field parsers.ParserNode) (bool, error) {
		if field.Kind != "field_declaration" {
			return true, nil
		}

		decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(field))

		if nameNode, ok := field.ChildByFieldName("name"); ok && hasSourceWidth(nameNode) {
			decl.Names = append(decl.Names, ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent()))
		}

		if typeNode, ok := field.ChildByFieldName("type"); ok {
			decl.Type = typeExpression(typeNode)
		}

		if len(decl.Names) > 0 || decl.Type != nil {
			fn.AppendChild(decl)
		}

		return true, nil
	})
}

func appendEnumVariants(fn *ast.ASTFuncExpression, list parsers.ParserNode) {
	list.IterateChildren(func(variant parsers.ParserNode) (bool, error) {
		if variant.Kind != "enum_variant" {
			return true, nil
		}

		decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(variant))

		if nameNode, ok := variant.ChildByFieldName("name"); ok && hasSourceWidth(nameNode) {
			decl.Names = append(decl.Names, ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent()))
		}

		if bodyNode, ok := variant.ChildByFieldName("body"); ok {
			decl.Type = foldTypes(collectTypes(bodyNode))
		}

		if len(decl.Names) > 0 || decl.Type != nil {
			fn.AppendChild(decl)
		}

		return true, nil
	})
}

// selfTypeHeadName finds the identifier naming the self type of an impl block
// (`Widget` in `impl Widget`, `Stack` in `impl<T> Stack<T>`)
func selfTypeHeadName(typeNode parsers.ParserNode) (parsers.ParserNode, bool) {
	switch typeNode.Kind {
	case "type_identifier":
		return typeNode, true
	case "scoped_type_identifier":
		return typeNode.ChildByFieldName("name")
	case "generic_type":
		if headNode, ok := typeNode.ChildByFieldName("type"); ok {
			return selfTypeHeadName(headNode)
		}
	}

	var zero parsers.ParserNode
	return zero, false
}

// newUseImport anchors an import on the last path segment so the LSP
// definition query lands on the item being imported: the first segment of a
// use path is usually std, crate or super, which resolves to a crate root
// rather than the file the import brings in
func newUseImport(pathNode parsers.ParserNode, prefix string) *ast.ASTImportStatement {
	leaf := pathNode

	if pathNode.Kind == "scoped_identifier" {
		nameNode, ok := pathNode.ChildByFieldName("name")

		if !ok {
			return nil
		}

		leaf = nameNode
	}

	if !hasSourceWidth(leaf) {
		return nil
	}

	imp := ast.NewASTImportStatement(ast.NewASTNodeContainer(leaf))
	imp.Reference = ast.NewASTSymbol(ast.NewASTNodeContainer(leaf), prefix+pathNode.GetTextContent())

	return imp
}

func emitUseTree(trx *parsers.TransformContext, node parsers.ParserNode, prefix string) error {
	switch node.Kind {
	case "identifier", "scoped_identifier", "self", "crate", "super":
		if imp := newUseImport(node, prefix); imp != nil {
			return trx.Emit(imp)
		}

		return nil

	case "use_as_clause":
		pathNode, ok := node.ChildByFieldName("path")

		if !ok {
			return nil
		}

		imp := newUseImport(pathNode, prefix)

		if imp == nil {
			return nil
		}

		if aliasNode, ok := node.ChildByFieldName("alias"); ok && hasSourceWidth(aliasNode) {
			imp.Namespace = ast.NewASTSymbol(ast.NewASTNodeContainer(aliasNode), aliasNode.GetTextContent())
		}

		return trx.Emit(imp)

	case "scoped_use_list":
		newPrefix := prefix

		if pathNode, ok := node.ChildByFieldName("path"); ok {
			newPrefix = prefix + pathNode.GetTextContent() + "::"
		}

		if listNode, ok := node.ChildByFieldName("list"); ok {
			return emitUseTree(trx, listNode, newPrefix)
		}

		return nil

	case "use_list":
		return node.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
			if err := emitUseTree(trx, pn, prefix); err != nil {
				return false, err
			}
			return true, nil
		})

	case "use_wildcard":
		// The wildcard's path is its only named child; the glob itself is not
		// resolvable, the module it opens is
		var err error

		node.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
			err = emitUseTree(trx, pn, prefix)
			return false, err
		})

		return err
	}

	return nil
}

// calleeSymbols reads the callee of a call: symbol on the called name,
// namespace on a simple qualifier
func calleeSymbols(fnNode parsers.ParserNode) (*ast.ASTSymbol, *ast.ASTSymbol) {
	switch fnNode.Kind {
	case "identifier":
		if !hasSourceWidth(fnNode) {
			return nil, nil
		}

		return ast.NewASTSymbol(ast.NewASTNodeContainer(fnNode), fnNode.GetTextContent()), nil

	case "scoped_identifier":
		nameNode, ok := fnNode.ChildByFieldName("name")

		if !ok || !hasSourceWidth(nameNode) {
			return nil, nil
		}

		sym := ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())

		if pathNode, ok := fnNode.ChildByFieldName("path"); ok && pathNode.Kind == "identifier" && hasSourceWidth(pathNode) {
			return sym, ast.NewASTSymbol(ast.NewASTNodeContainer(pathNode), pathNode.GetTextContent())
		}

		return sym, nil

	case "field_expression":
		// Skips over chained calls (e.g. x.y().z()), those are handled by
		// walking children into the emitted call
		fieldNode, ok := fnNode.ChildByFieldName("field")

		if !ok || !hasSourceWidth(fieldNode) {
			return nil, nil
		}

		sym := ast.NewASTSymbol(ast.NewASTNodeContainer(fieldNode), fieldNode.GetTextContent())

		if valueNode, ok := fnNode.ChildByFieldName("value"); ok && valueNode.Kind == "identifier" && hasSourceWidth(valueNode) {
			return sym, ast.NewASTSymbol(ast.NewASTNodeContainer(valueNode), valueNode.GetTextContent())
		}

		return sym, nil

	case "generic_function":
		// Turbofish (`parse::<i32>`) wraps the real callee
		if innerNode, ok := fnNode.ChildByFieldName("function"); ok {
			return calleeSymbols(innerNode)
		}
	}

	return nil, nil
}

func RustTransformer(ctx context.Context, trx *parsers.TransformContext, node parsers.ParserNode) error {
	switch node.Kind {
	case "use_declaration":
		if argNode, ok := node.ChildByFieldName("argument"); ok {
			return emitUseTree(trx, argNode, "")
		}

		return nil

	case "extern_crate_declaration":
		if nameNode, ok := node.ChildByFieldName("name"); ok {
			if imp := newUseImport(nameNode, ""); imp != nil {
				return trx.Emit(imp)
			}
		}

		return nil

	case "mod_item":
		bodyNode, ok := node.ChildByFieldName("body")

		// A bodyless `mod name;` includes another file, which is where its
		// definition resolves
		if !ok {
			if nameNode, ok := node.ChildByFieldName("name"); ok {
				if imp := newUseImport(nameNode, ""); imp != nil {
					return trx.Emit(imp)
				}
			}

			return nil
		}

		fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))

		if nameNode, ok := node.ChildByFieldName("name"); ok && hasSourceWidth(nameNode) {
			fn.Name = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
		}

		if hasSourceWidth(bodyNode) {
			fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(bodyNode))
		}

		if err := trx.Emit(fn); err != nil {
			return err
		}

		return trx.WalkChildrenInto(ctx, fn)

	case "function_item", "function_signature_item":
		fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))

		if nameNode, ok := node.ChildByFieldName("name"); ok && hasSourceWidth(nameNode) {
			fn.Name = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
		}

		appendTypeParameters(fn, node)

		if paramsNode, ok := node.ChildByFieldName("parameters"); ok {
			appendParameters(fn, paramsNode)
		}

		fn.ReturnType = returnType(node)

		// Functions auto-assign a block but we narrow it to the body so the
		// function stays visible to range queries; a signature item has no
		// body and keeps the shadow deliberately
		if bodyNode, ok := node.ChildByFieldName("body"); ok && hasSourceWidth(bodyNode) {
			fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(bodyNode))
		}

		if err := trx.Emit(fn); err != nil {
			return err
		}

		return trx.WalkChildrenInto(ctx, fn)

	case "closure_expression":
		fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))

		if paramsNode, ok := node.ChildByFieldName("parameters"); ok {
			appendParameters(fn, paramsNode)
		}

		fn.ReturnType = returnType(node)

		if bodyNode, ok := node.ChildByFieldName("body"); ok && hasSourceWidth(bodyNode) {
			fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(bodyNode))
		}

		if err := trx.Emit(fn); err != nil {
			return err
		}

		return trx.WalkChildrenInto(ctx, fn)

	case "struct_item", "union_item":
		// Types map to FuncExpression on purpose: they carry the members
		// observations attach to and become observation-graph vertices like
		// any function
		fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))

		if nameNode, ok := node.ChildByFieldName("name"); ok && hasSourceWidth(nameNode) {
			fn.Name = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
		}

		appendTypeParameters(fn, node)

		if bodyNode, ok := node.ChildByFieldName("body"); ok && hasSourceWidth(bodyNode) {
			fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(bodyNode))

			switch bodyNode.Kind {
			case "field_declaration_list":
				appendStructFields(fn, bodyNode)
			case "ordered_field_declaration_list":
				decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(bodyNode))
				decl.Type = foldTypes(collectTypes(bodyNode))

				if decl.Type != nil {
					fn.AppendChild(decl)
				}
			}
		}

		return trx.Emit(fn)

	case "enum_item":
		fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))

		if nameNode, ok := node.ChildByFieldName("name"); ok && hasSourceWidth(nameNode) {
			fn.Name = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
		}

		appendTypeParameters(fn, node)

		if bodyNode, ok := node.ChildByFieldName("body"); ok && hasSourceWidth(bodyNode) {
			fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(bodyNode))
			appendEnumVariants(fn, bodyNode)
		}

		return trx.Emit(fn)

	case "trait_item":
		fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))

		if nameNode, ok := node.ChildByFieldName("name"); ok && hasSourceWidth(nameNode) {
			fn.Name = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
		}

		appendTypeParameters(fn, node)

		if bodyNode, ok := node.ChildByFieldName("body"); ok && hasSourceWidth(bodyNode) {
			fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(bodyNode))
		}

		// Supertraits hang off the trait so their definitions join the graph
		if boundsNode, ok := node.ChildByFieldName("bounds"); ok {
			if texpr := foldTypes(collectTypes(boundsNode)); texpr != nil {
				fn.AppendChild(texpr)
			}
		}

		if err := trx.Emit(fn); err != nil {
			return err
		}

		return trx.WalkChildrenInto(ctx, fn)

	case "impl_item":
		fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))

		// The impl is named after its self type; the implemented trait hangs
		// off it as a type reference so its definition joins the graph
		if typeNode, ok := node.ChildByFieldName("type"); ok {
			if nameNode, ok := selfTypeHeadName(typeNode); ok && hasSourceWidth(nameNode) {
				fn.Name = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
			}
		}

		appendTypeParameters(fn, node)

		if bodyNode, ok := node.ChildByFieldName("body"); ok && hasSourceWidth(bodyNode) {
			fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(bodyNode))
		}

		if traitNode, ok := node.ChildByFieldName("trait"); ok {
			if texpr := typeExpression(traitNode); texpr != nil {
				fn.AppendChild(texpr)
			}
		}

		if err := trx.Emit(fn); err != nil {
			return err
		}

		return trx.WalkChildrenInto(ctx, fn)

	case "associated_type":
		// A trait's associated type declares only a name; the impl's type_item
		// carries the assignment
		nameNode, ok := node.ChildByFieldName("name")

		if !ok || !hasSourceWidth(nameNode) {
			return nil
		}

		decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(node))
		decl.Names = []*ast.ASTSymbol{ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())}

		return trx.Emit(decl)

	case "type_item":
		// A member-less alias needs only a declaration on its name; its type
		// parameters join the names so definitions resolving onto them hold
		decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(node))

		if nameNode, ok := node.ChildByFieldName("name"); ok && hasSourceWidth(nameNode) {
			decl.Names = append(decl.Names, ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent()))
		}

		decl.Names = append(decl.Names, typeParameterNames(node)...)

		if typeNode, ok := node.ChildByFieldName("type"); ok {
			decl.Type = typeExpression(typeNode)
		}

		if len(decl.Names) == 0 && decl.Type == nil {
			return nil
		}

		return trx.Emit(decl)

	case "const_item", "static_item":
		decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(node))

		if nameNode, ok := node.ChildByFieldName("name"); ok && hasSourceWidth(nameNode) {
			decl.Names = append(decl.Names, ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent()))
		}

		if typeNode, ok := node.ChildByFieldName("type"); ok {
			decl.Type = typeExpression(typeNode)
		}

		if err := trx.Emit(decl); err != nil {
			return err
		}

		return trx.WalkChildrenInto(ctx, decl)

	case "let_declaration":
		decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(node))

		if patternNode, ok := node.ChildByFieldName("pattern"); ok {
			decl.Names = collectPatternNames(patternNode)
		}

		if typeNode, ok := node.ChildByFieldName("type"); ok {
			decl.Type = typeExpression(typeNode)
		}

		if err := trx.Emit(decl); err != nil {
			return err
		}

		// RHS: walk into decl so call expressions on the right-hand side land
		// in Virtual
		return trx.WalkChildrenInto(ctx, decl)

	case "for_expression":
		if patternNode, ok := node.ChildByFieldName("pattern"); ok {
			if err := emitBindingDeclaration(trx, patternNode); err != nil {
				return err
			}
		}

		return trx.WalkChildren(ctx)

	case "let_condition":
		if patternNode, ok := node.ChildByFieldName("pattern"); ok {
			if err := emitBindingDeclaration(trx, patternNode); err != nil {
				return err
			}
		}

		return trx.WalkChildren(ctx)

	case "match_arm":
		if patternNode, ok := node.ChildByFieldName("pattern"); ok {
			if err := emitBindingDeclaration(trx, patternNode); err != nil {
				return err
			}
		}

		return trx.WalkChildren(ctx)

	case "struct_expression":
		// Construction is the call-shaped use of a type; the name resolves to
		// the type's definition
		if nameNode, ok := node.ChildByFieldName("name"); ok {
			if texpr := typeExpression(nameNode); texpr != nil {
				if err := trx.Emit(texpr); err != nil {
					return err
				}
			}
		}

		return trx.WalkChildren(ctx)

	case "call_expression":
		callExpr := ast.NewASTCallExpression(ast.NewASTNodeContainer(node))

		if fnNode, ok := node.ChildByFieldName("function"); ok {
			callExpr.Symbol, callExpr.Namespace = calleeSymbols(fnNode)
		}

		// No identifiable callee (e.g. a closure call); its children still
		// index under the enclosing block
		if callExpr.Symbol == nil {
			return trx.WalkChildren(ctx)
		}

		if err := trx.Emit(callExpr); err != nil {
			return err
		}

		// Handles chained operands and nested calls
		return trx.WalkChildrenInto(ctx, callExpr)

	case "macro_invocation":
		// A macro call resolves like a function call; its arguments are raw
		// tokens the grammar does not parse, so they contribute no edges
		callExpr := ast.NewASTCallExpression(ast.NewASTNodeContainer(node))

		if macroNode, ok := node.ChildByFieldName("macro"); ok {
			callExpr.Symbol, callExpr.Namespace = calleeSymbols(macroNode)
		}

		if callExpr.Symbol == nil {
			return trx.WalkChildren(ctx)
		}

		return trx.Emit(callExpr)

	case "macro_definition":
		fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))

		if nameNode, ok := node.ChildByFieldName("name"); ok && hasSourceWidth(nameNode) {
			fn.Name = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
		}

		return trx.Emit(fn)

	case "line_comment", "block_comment":
		return nil

	default:
		return trx.WalkChildren(ctx)
	}
}

type RustLanguageSupportDefinition struct{}

func NewRustLanguageSupportDefinition() *RustLanguageSupportDefinition {
	return &RustLanguageSupportDefinition{}
}

var (
	// rust-analyzer resolves std/core/alloc into the rust-src component of the
	// active toolchain
	rustStdlibRE = regexp.MustCompile(`/lib/rustlib/src/rust/library/`)
	// Crates from the registry cache (`~/.cargo/registry/src/index.crates.io-…/`),
	// git dependencies, and vendored sources
	cargoRegistryRE = regexp.MustCompile(`/(registry/src|git/checkouts)/`)
	vendorRE        = regexp.MustCompile(`(^|/)vendor/`)
)

func (rlsd *RustLanguageSupportDefinition) ClassifyImportType(s *common.Source) common.DependencyType {
	p := filepath.ToSlash(filepath.Clean(s.Path))

	if rustStdlibRE.MatchString(p) {
		return common.StandardLibraryDependency
	}

	if cargoRegistryRE.MatchString(p) || vendorRE.MatchString(p) {
		return common.PackageDependency
	}

	return common.LocalDependency
}

func (rlsd *RustLanguageSupportDefinition) Parse(ctx context.Context, src *common.Source, tree *tree_sitter.Tree) (*ast.ASTModule, error) {
	name := strings.TrimSuffix(filepath.Base(src.Path), filepath.Ext(src.Path))

	root := ast.NewASTModule(ast.NewASTNodeContainer(
		parsers.NewParserNode(src, tree.RootNode()),
	), name)

	if err := parsers.WalkTransformTree(ctx, src, tree, root, RustTransformer); err != nil {
		return nil, err
	}

	return root, nil
}

func (rlsd *RustLanguageSupportDefinition) GetLanguageID() string {
	return "rust"
}

func (rlsd *RustLanguageSupportDefinition) GetTreeSitterLanguage() *tree_sitter.Language {
	return tree_sitter.NewLanguage(tree_sitter_rust.Language())
}
