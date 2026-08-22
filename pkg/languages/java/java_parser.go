package languages_java

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/lsp"
	"github.com/schahriar/captn/pkg/parsers"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_java "github.com/tree-sitter/tree-sitter-java/bindings/go"
)

// hasSourceWidth reports whether a node spans any bytes; tree-sitter error
// recovery produces zero-width nodes that must never become AST nodes (the
// interval index rejects empty ranges)
func hasSourceWidth(node parsers.ParserNode) bool {
	return node.Range.StartByte < node.Range.EndByte
}

// typeExpression maps a type onto the identifier a definition resolves to.
// Nameless composites (arrays, wildcards) fold flat: the first named type
// inside is the head and the rest are its arguments. Primitive types (int,
// void) are keywords with no definition site and yield nil.
func typeExpression(node parsers.ParserNode) *ast.ASTTypeExpression {
	switch node.Kind {
	case "type_identifier":
		if !hasSourceWidth(node) {
			return nil
		}

		return ast.NewASTTypeExpression(ast.NewASTNodeContainer(node), node.GetTextContent())

	case "scoped_type_identifier":
		var segments []parsers.ParserNode

		node.IterateChildren(func(child parsers.ParserNode) (bool, error) {
			if child.Kind == "type_identifier" && hasSourceWidth(child) {
				segments = append(segments, child)
			}
			return true, nil
		})

		if len(segments) == 0 {
			return nil
		}

		nameNode := segments[len(segments)-1]
		texpr := ast.NewASTTypeExpression(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())

		if len(segments) > 1 {
			scope := segments[len(segments)-2]
			texpr.Namespace = ast.NewASTSymbol(ast.NewASTNodeContainer(scope), scope.GetTextContent())
		}

		return texpr

	case "generic_type":
		var head *ast.ASTTypeExpression
		var args parsers.ParserNode
		hasArgs := false

		node.IterateChildren(func(child parsers.ParserNode) (bool, error) {
			switch child.Kind {
			case "type_identifier", "scoped_type_identifier":
				head = typeExpression(child)
			case "type_arguments":
				args = child
				hasArgs = true
			}
			return true, nil
		})

		if head == nil {
			return nil
		}

		if hasArgs {
			head.Arguments = append(head.Arguments, collectTypes(args)...)
		}

		return head

	case "array_type":
		if element, ok := node.ChildByFieldName("element"); ok {
			return typeExpression(element)
		}

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
		case "type_identifier", "scoped_type_identifier", "generic_type", "array_type":
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

func appendParameters(fn *ast.ASTFuncExpression, list parsers.ParserNode) {
	list.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
		switch pn.Kind {
		case "formal_parameter", "spread_parameter":
			var texpr *ast.ASTTypeExpression

			if typeNode, ok := pn.ChildByFieldName("type"); ok {
				texpr = typeExpression(typeNode)
			}

			if nameNode, ok := pn.ChildByFieldName("name"); ok && hasSourceWidth(nameNode) {
				sym := ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
				fn.Arguments = append(fn.Arguments, ast.NewASTFuncArgument(ast.NewASTNodeContainer(pn), sym, texpr))
			}

		case "identifier":
			// Lambda inferred parameters carry bare identifiers
			if hasSourceWidth(pn) {
				sym := ast.NewASTSymbol(ast.NewASTNodeContainer(pn), pn.GetTextContent())
				fn.Arguments = append(fn.Arguments, ast.NewASTFuncArgument(ast.NewASTNodeContainer(pn), sym, nil))
			}
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

		nameNode, ok := pn.GetNthChildByKind("type_identifier", 0)

		if !ok || !hasSourceWidth(nameNode) {
			return true, nil
		}

		sym := ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())

		var bound *ast.ASTTypeExpression

		if boundNode, ok := pn.GetNthChildByKind("type_bound", 0); ok {
			bound = typeExpression(boundNode)
		}

		fn.Arguments = append(fn.Arguments, ast.NewASTFuncArgument(ast.NewASTNodeContainer(pn), sym, bound))

		return true, nil
	})
}

// calleeSymbols maps the constructed type of a `new` expression onto the class
// identifier a definition resolves to, with its scope as namespace
func calleeSymbols(typeNode parsers.ParserNode) (*ast.ASTSymbol, *ast.ASTSymbol) {
	switch typeNode.Kind {
	case "type_identifier":
		if !hasSourceWidth(typeNode) {
			return nil, nil
		}

		return ast.NewASTSymbol(ast.NewASTNodeContainer(typeNode), typeNode.GetTextContent()), nil

	case "scoped_type_identifier":
		var segments []parsers.ParserNode

		typeNode.IterateChildren(func(child parsers.ParserNode) (bool, error) {
			if child.Kind == "type_identifier" && hasSourceWidth(child) {
				segments = append(segments, child)
			}
			return true, nil
		})

		if len(segments) == 0 {
			return nil, nil
		}

		nameNode := segments[len(segments)-1]
		sym := ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())

		var namespace *ast.ASTSymbol

		if len(segments) > 1 {
			scope := segments[len(segments)-2]
			namespace = ast.NewASTSymbol(ast.NewASTNodeContainer(scope), scope.GetTextContent())
		}

		return sym, namespace

	case "generic_type":
		var sym, namespace *ast.ASTSymbol

		typeNode.IterateChildren(func(child parsers.ParserNode) (bool, error) {
			switch child.Kind {
			case "type_identifier", "scoped_type_identifier":
				sym, namespace = calleeSymbols(child)
			}
			return true, nil
		})

		return sym, namespace
	}

	return nil, nil
}

func emitDeclaration(trx *parsers.TransformContext, ctx context.Context, node parsers.ParserNode) error {
	decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(node))

	if typeNode, ok := node.ChildByFieldName("type"); ok {
		decl.Type = typeExpression(typeNode)
	}

	node.IterateChildrenByFieldName("declarator", func(dn parsers.ParserNode) (bool, error) {
		if nameNode, ok := dn.ChildByFieldName("name"); ok && hasSourceWidth(nameNode) {
			decl.Names = append(decl.Names, ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent()))
		}
		return true, nil
	})

	if len(decl.Names) == 0 && decl.Type == nil {
		return trx.WalkChildren(ctx)
	}

	if err := trx.Emit(decl); err != nil {
		return err
	}

	// Initializers may hold calls or lambdas; walk into decl so they land in
	// Virtual
	return trx.WalkChildrenInto(ctx, decl)
}

// emitBindingDeclaration emits an ASTDeclaration for a single bound name
// (loop variable, catch parameter, try resource) so LSP-resolved definition
// ranges always contain a symbol
func emitBindingDeclaration(trx *parsers.TransformContext, container parsers.ParserNode, nameNode parsers.ParserNode, typeNode *parsers.ParserNode) error {
	if !hasSourceWidth(nameNode) {
		return nil
	}

	decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(container))
	decl.Names = append(decl.Names, ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent()))

	if typeNode != nil {
		decl.Type = typeExpression(*typeNode)
	}

	return trx.Emit(decl)
}

func emitFuncExpression(trx *parsers.TransformContext, ctx context.Context, node parsers.ParserNode) error {
	fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))

	if nameNode, ok := node.ChildByFieldName("name"); ok && hasSourceWidth(nameNode) {
		fn.Name = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
	}

	appendTypeParameters(fn, node)

	// The return type; a bodyless method (interface member, abstract or
	// annotation element) keeps its wide auto-assigned block deliberately, a
	// definition still resolves onto its name
	if typeNode, ok := node.ChildByFieldName("type"); ok {
		fn.ReturnType = typeExpression(typeNode)
	}

	if bodyNode, ok := node.ChildByFieldName("body"); ok && hasSourceWidth(bodyNode) {
		fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(bodyNode))
	}

	if err := trx.Emit(fn); err != nil {
		return err
	}

	return trx.WalkChildrenInto(ctx, fn)
}

func JavaTransformer(ctx context.Context, trx *parsers.TransformContext, node parsers.ParserNode) error {
	switch node.Kind {
	case "package_declaration":
		node.IterateChildren(func(child parsers.ParserNode) (bool, error) {
			switch child.Kind {
			case "scoped_identifier", "identifier":
				trx.Root.Name = child.GetTextContent()
			}
			return true, nil
		})

		return nil

	case "import_declaration":
		// The import node anchors on the last path segment (the class or
		// static member): jdtls resolves a definition there, never on the
		// leading package segments or the import keyword
		pathNode, ok := node.GetNthChildByKind("scoped_identifier", 0)

		if !ok {
			pathNode, ok = node.GetNthChildByKind("identifier", 0)
		}

		if !ok || !hasSourceWidth(pathNode) {
			return nil
		}

		anchor := pathNode

		if nameNode, ok := pathNode.ChildByFieldName("name"); ok && hasSourceWidth(nameNode) {
			anchor = nameNode
		}

		imp := ast.NewASTImportStatement(ast.NewASTNodeContainer(anchor))
		imp.Reference = ast.NewASTSymbol(ast.NewASTNodeContainer(pathNode), pathNode.GetTextContent())

		return trx.Emit(imp)

	case "class_declaration", "interface_declaration", "enum_declaration",
		"record_declaration", "annotation_type_declaration":
		// Types map to FuncExpression on purpose: a definition of a
		// constructor call or type reference lands on the type name, and the
		// type becomes an observation vertex holding its members
		return emitFuncExpression(trx, ctx, node)

	case "method_declaration", "constructor_declaration",
		"compact_constructor_declaration", "annotation_type_element_declaration":
		return emitFuncExpression(trx, ctx, node)

	case "lambda_expression":
		fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))

		// Expression bodies narrow onto the expression node; the collision
		// only shadows the block and stays benign
		if bodyNode, ok := node.ChildByFieldName("body"); ok && hasSourceWidth(bodyNode) {
			fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(bodyNode))
		}

		if paramsNode, ok := node.ChildByFieldName("parameters"); ok && paramsNode.Kind == "identifier" && hasSourceWidth(paramsNode) {
			sym := ast.NewASTSymbol(ast.NewASTNodeContainer(paramsNode), paramsNode.GetTextContent())
			fn.Arguments = append(fn.Arguments, ast.NewASTFuncArgument(ast.NewASTNodeContainer(paramsNode), sym, nil))
		}

		if err := trx.Emit(fn); err != nil {
			return err
		}

		return trx.WalkChildrenInto(ctx, fn)

	case "formal_parameters", "inferred_parameters":
		fn, ok := trx.Parent.(*ast.ASTFuncExpression)

		if !ok {
			return nil
		}

		// A record's parameters are its components; a parenthesized lambda
		// lists parameters the same way a method does
		switch kind, _ := node.ParentKind(); kind {
		case "method_declaration", "constructor_declaration", "record_declaration", "lambda_expression":
			appendParameters(fn, node)
		}

		return nil

	case "field_declaration", "local_variable_declaration":
		return emitDeclaration(trx, ctx, node)

	case "enhanced_for_statement":
		if nameNode, ok := node.ChildByFieldName("name"); ok {
			typeNode, hasType := node.ChildByFieldName("type")

			var tn *parsers.ParserNode
			if hasType {
				tn = &typeNode
			}

			if err := emitBindingDeclaration(trx, nameNode, nameNode, tn); err != nil {
				return err
			}
		}

		return trx.WalkChildren(ctx)

	case "catch_formal_parameter":
		nameNode, ok := node.GetNthChildByKind("identifier", 0)

		if !ok {
			return trx.WalkChildren(ctx)
		}

		var tn *parsers.ParserNode

		if typeNode, hasType := node.GetNthChildByKind("catch_type", 0); hasType {
			tn = &typeNode
		}

		return emitBindingDeclaration(trx, node, nameNode, tn)

	case "resource":
		nameNode, ok := node.ChildByFieldName("name")

		// A resource may also name an existing variable; nothing declares there
		if !ok {
			return trx.WalkChildren(ctx)
		}

		decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(node))
		decl.Names = append(decl.Names, ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent()))

		if typeNode, hasType := node.ChildByFieldName("type"); hasType {
			decl.Type = typeExpression(typeNode)
		}

		if err := trx.Emit(decl); err != nil {
			return err
		}

		return trx.WalkChildrenInto(ctx, decl)

	case "enum_constant":
		nameNode, ok := node.ChildByFieldName("name")

		if !ok || !hasSourceWidth(nameNode) {
			return trx.WalkChildren(ctx)
		}

		decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(node))
		decl.Names = append(decl.Names, ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent()))

		if err := trx.Emit(decl); err != nil {
			return err
		}

		// Constructor arguments and constant bodies walk into the declaration
		return trx.WalkChildrenInto(ctx, decl)

	case "method_invocation":
		callExpr := ast.NewASTCallExpression(ast.NewASTNodeContainer(node))

		if objectNode, ok := node.ChildByFieldName("object"); ok && objectNode.Kind == "identifier" && hasSourceWidth(objectNode) {
			callExpr.Namespace = ast.NewASTSymbol(ast.NewASTNodeContainer(objectNode), objectNode.GetTextContent())
		}

		if nameNode, ok := node.ChildByFieldName("name"); ok && hasSourceWidth(nameNode) {
			callExpr.Symbol = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
		}

		// No identifiable callee; children still index under the enclosing
		// block
		if callExpr.Symbol == nil {
			return trx.WalkChildren(ctx)
		}

		if err := trx.Emit(callExpr); err != nil {
			return err
		}

		// Handles chained and nested calls
		return trx.WalkChildrenInto(ctx, callExpr)

	case "object_creation_expression":
		callExpr := ast.NewASTCallExpression(ast.NewASTNodeContainer(node))

		if typeNode, ok := node.ChildByFieldName("type"); ok {
			callExpr.Symbol, callExpr.Namespace = calleeSymbols(typeNode)
		}

		if callExpr.Symbol == nil {
			return trx.WalkChildren(ctx)
		}

		if err := trx.Emit(callExpr); err != nil {
			return err
		}

		return trx.WalkChildrenInto(ctx, callExpr)

	default:
		return trx.WalkChildren(ctx)
	}
}

type JavaLanguageSupportDefinition struct{}

func NewJavaLanguageSupportDefinition() *JavaLanguageSupportDefinition {
	return &JavaLanguageSupportDefinition{}
}

func (jlsd *JavaLanguageSupportDefinition) ClassifyImportType(s *common.Source) common.DependencyType {
	return common.LocalDependency
}

func (jlsd *JavaLanguageSupportDefinition) GetLSPServerRequirement() lsp.ServerRequirement {
	// banstructlit:ignore
	return lsp.ServerRequirement{
		Name:           jdtlsName,
		InstallCommand: jdtlsInstallCommand,
		Locate:         jdtlsPath,
	}
}

func (jlsd *JavaLanguageSupportDefinition) Parse(ctx context.Context, src *common.Source, tree *tree_sitter.Tree) (*ast.ASTModule, error) {
	name := strings.TrimSuffix(filepath.Base(src.Path), filepath.Ext(src.Path))

	root := ast.NewASTModule(ast.NewASTNodeContainer(
		parsers.NewParserNode(src, tree.RootNode()),
	), name)

	if err := parsers.WalkTransformTree(ctx, src, tree, root, JavaTransformer); err != nil {
		return nil, err
	}

	return root, nil
}

func (jlsd *JavaLanguageSupportDefinition) GetLanguageID() string {
	return "java"
}

func (jlsd *JavaLanguageSupportDefinition) GetTreeSitterLanguage() *tree_sitter.Language {
	return tree_sitter.NewLanguage(tree_sitter_java.Language())
}
