package languages_php

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
	tree_sitter_php "github.com/tree-sitter/tree-sitter-php/bindings/go"
)

// hasSourceWidth reports whether a node spans any bytes; tree-sitter error
// recovery produces zero-width nodes that must never become AST nodes (the
// interval index rejects empty ranges)
func hasSourceWidth(node parsers.ParserNode) bool {
	return node.Range.StartByte < node.Range.EndByte
}

// typeExpression maps a type onto the identifier a definition resolves to.
// Composites (nullable, union, intersection) fold flat: the first named type
// is the head and the rest are its arguments.
func typeExpression(node parsers.ParserNode) *ast.ASTTypeExpression {
	switch node.Kind {
	case "name", "primitive_type":
		if !hasSourceWidth(node) {
			return nil
		}

		return ast.NewASTTypeExpression(ast.NewASTNodeContainer(node), node.GetTextContent())

	case "qualified_name":
		nameNode, ok := node.GetNthChildByKind("name", 0)

		if !ok || !hasSourceWidth(nameNode) {
			return nil
		}

		texpr := ast.NewASTTypeExpression(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())

		if prefixNode, ok := node.ChildByFieldName("prefix"); ok && prefixNode.Kind == "namespace_name" {
			texpr.Namespace = ast.NewASTSymbol(ast.NewASTNodeContainer(prefixNode), prefixNode.GetTextContent())
		}

		return texpr

	case "relative_name", "named_type", "optional_type":
		var texpr *ast.ASTTypeExpression

		node.IterateChildren(func(child parsers.ParserNode) (bool, error) {
			if texpr = typeExpression(child); texpr != nil {
				return false, nil
			}
			return true, nil
		})

		return texpr
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
		case "name", "primitive_type", "qualified_name", "relative_name", "named_type", "optional_type":
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
		case "simple_parameter", "variadic_parameter", "property_promotion_parameter":
			nameNode, ok := pn.ChildByFieldName("name")

			if !ok || nameNode.Kind != "variable_name" || !hasSourceWidth(nameNode) {
				return true, nil
			}

			sym := ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())

			var texpr *ast.ASTTypeExpression
			if typeNode, ok := pn.ChildByFieldName("type"); ok {
				texpr = typeExpression(typeNode)
			}

			fn.Arguments = append(fn.Arguments, ast.NewASTFuncArgument(ast.NewASTNodeContainer(pn), sym, texpr))
		}

		return true, nil
	})
}

// emitFuncExpression narrows the block to the body so the function stays
// visible to range queries. A bodyless declaration (abstract or interface
// method) keeps the shadow deliberately.
func emitFuncExpression(ctx context.Context, trx *parsers.TransformContext, node parsers.ParserNode, fn *ast.ASTFuncExpression) error {
	if bodyNode, ok := node.ChildByFieldName("body"); ok && hasSourceWidth(bodyNode) {
		fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(bodyNode))
	}

	if err := trx.Emit(fn); err != nil {
		return err
	}

	return trx.WalkChildrenInto(ctx, fn)
}

// callSymbol maps a callee node onto the identifier a definition resolves to
func callSymbol(callExpr *ast.ASTCallExpression, callee parsers.ParserNode) {
	switch callee.Kind {
	case "name":
		if hasSourceWidth(callee) {
			callExpr.Symbol = ast.NewASTSymbol(ast.NewASTNodeContainer(callee), callee.GetTextContent())
		}

	case "qualified_name":
		if nameNode, ok := callee.GetNthChildByKind("name", 0); ok && hasSourceWidth(nameNode) {
			callExpr.Symbol = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
		}

		if prefixNode, ok := callee.ChildByFieldName("prefix"); ok && prefixNode.Kind == "namespace_name" {
			callExpr.Namespace = ast.NewASTSymbol(ast.NewASTNodeContainer(prefixNode), prefixNode.GetTextContent())
		}
	}
}

func PHPTransformer(ctx context.Context, trx *parsers.TransformContext, node parsers.ParserNode) error {
	switch node.Kind {
	case "namespace_definition":
		if nameNode, ok := node.ChildByFieldName("name"); ok {
			trx.Root.Name = nameNode.GetTextContent()
		}

		return trx.WalkChildren(ctx)

	case "namespace_use_clause":
		// One clause per imported name; the group form (use App\{A, B})
		// reaches its clauses through the default walk
		var nameNode parsers.ParserNode
		found := false

		node.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
			if pn.Kind == "name" || pn.Kind == "qualified_name" {
				nameNode = pn
				found = true
				return false, nil
			}
			return true, nil
		})

		if !found || !hasSourceWidth(nameNode) {
			return nil
		}

		imp := ast.NewASTImportStatement(ast.NewASTNodeContainer(nameNode))
		imp.Reference = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())

		if aliasNode, ok := node.ChildByFieldName("alias"); ok && hasSourceWidth(aliasNode) {
			imp.Namespace = ast.NewASTSymbol(ast.NewASTNodeContainer(aliasNode), aliasNode.GetTextContent())
		}

		return trx.Emit(imp)

	case "require_expression", "require_once_expression", "include_expression", "include_once_expression":
		// Only a plain string path is an import; a computed path
		// (__DIR__ . '/x.php') has no resolvable text
		strNode, ok := node.GetNthChildByKind("string", 0)

		if !ok {
			return trx.WalkChildren(ctx)
		}

		contentNode, ok := strNode.GetNthChildByKind("string_content", 0)

		if !ok || !hasSourceWidth(contentNode) {
			return trx.WalkChildren(ctx)
		}

		imp := ast.NewASTImportStatement(ast.NewASTNodeContainer(contentNode))
		imp.Reference = ast.NewASTSymbol(ast.NewASTNodeContainer(contentNode), contentNode.GetTextContent())

		return trx.Emit(imp)

	case "function_definition", "method_declaration":
		fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))

		if nameNode, ok := node.ChildByFieldName("name"); ok && hasSourceWidth(nameNode) {
			fn.Name = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
		}

		if paramsNode, ok := node.ChildByFieldName("parameters"); ok {
			appendParameters(fn, paramsNode)
		}

		if retNode, ok := node.ChildByFieldName("return_type"); ok {
			fn.ReturnType = typeExpression(retNode)
		}

		return emitFuncExpression(ctx, trx, node, fn)

	case "class_declaration", "interface_declaration", "trait_declaration", "enum_declaration":
		// Types map to FuncExpression on purpose: LSP definitions of type
		// references land on their name, and each becomes an observation
		// vertex like any function
		fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))

		if nameNode, ok := node.ChildByFieldName("name"); ok && hasSourceWidth(nameNode) {
			fn.Name = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
		}

		if bodyNode, ok := node.ChildByFieldName("body"); ok && hasSourceWidth(bodyNode) {
			fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(bodyNode))
		}

		// extends and implements clauses become type references under the type
		node.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
			switch pn.Kind {
			case "base_clause", "class_interface_clause":
				for _, texpr := range collectTypes(pn) {
					fn.AppendChild(texpr)
				}
			}
			return true, nil
		})

		if err := trx.Emit(fn); err != nil {
			return err
		}

		return trx.WalkChildrenInto(ctx, fn)

	case "anonymous_function", "arrow_function":
		fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))

		if paramsNode, ok := node.ChildByFieldName("parameters"); ok {
			appendParameters(fn, paramsNode)
		}

		if retNode, ok := node.ChildByFieldName("return_type"); ok {
			fn.ReturnType = typeExpression(retNode)
		}

		return emitFuncExpression(ctx, trx, node, fn)

	case "assignment_expression":
		leftNode, ok := node.ChildByFieldName("left")

		if !ok || leftNode.Kind != "variable_name" || !hasSourceWidth(leftNode) {
			return trx.WalkChildren(ctx)
		}

		decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(node))
		decl.Names = []*ast.ASTSymbol{ast.NewASTSymbol(ast.NewASTNodeContainer(leftNode), leftNode.GetTextContent())}

		if err := trx.Emit(decl); err != nil {
			return err
		}

		// RHS: walk into decl so call expressions on the right-hand side
		// land in Virtual
		return trx.WalkChildrenInto(ctx, decl)

	case "const_declaration", "property_declaration":
		decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(node))

		node.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
			switch pn.Kind {
			case "const_element":
				if nameNode, ok := pn.GetNthChildByKind("name", 0); ok && hasSourceWidth(nameNode) {
					decl.Names = append(decl.Names, ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent()))
				}
			case "property_element":
				if nameNode, ok := pn.ChildByFieldName("name"); ok && hasSourceWidth(nameNode) {
					decl.Names = append(decl.Names, ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent()))
				}
			}
			return true, nil
		})

		if len(decl.Names) == 0 {
			return trx.WalkChildren(ctx)
		}

		if typeNode, ok := node.ChildByFieldName("type"); ok {
			decl.Type = typeExpression(typeNode)
		}

		if err := trx.Emit(decl); err != nil {
			return err
		}

		return trx.WalkChildrenInto(ctx, decl)

	case "function_call_expression":
		callExpr := ast.NewASTCallExpression(ast.NewASTNodeContainer(node))

		if fnNode, ok := node.ChildByFieldName("function"); ok {
			callSymbol(callExpr, fnNode)
		}

		// No identifiable callee (a variable or nested call); its children
		// still index under the enclosing block
		if callExpr.Symbol == nil {
			return trx.WalkChildren(ctx)
		}

		if err := trx.Emit(callExpr); err != nil {
			return err
		}

		return trx.WalkChildrenInto(ctx, callExpr)

	case "member_call_expression", "nullsafe_member_call_expression", "scoped_call_expression":
		callExpr := ast.NewASTCallExpression(ast.NewASTNodeContainer(node))

		if nameNode, ok := node.ChildByFieldName("name"); ok && nameNode.Kind == "name" && hasSourceWidth(nameNode) {
			callExpr.Symbol = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
		}

		receiverField := "object"
		if node.Kind == "scoped_call_expression" {
			receiverField = "scope"
		}

		if recvNode, ok := node.ChildByFieldName(receiverField); ok {
			switch recvNode.Kind {
			case "variable_name", "name":
				callExpr.Namespace = ast.NewASTSymbol(ast.NewASTNodeContainer(recvNode), recvNode.GetTextContent())
			}
		}

		if callExpr.Symbol == nil {
			return trx.WalkChildren(ctx)
		}

		if err := trx.Emit(callExpr); err != nil {
			return err
		}

		// Handles chained receivers and argument calls
		return trx.WalkChildrenInto(ctx, callExpr)

	case "object_creation_expression":
		// new Widget(...) resolves on the class name like any call
		callExpr := ast.NewASTCallExpression(ast.NewASTNodeContainer(node))

		node.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
			switch pn.Kind {
			case "name", "qualified_name":
				callSymbol(callExpr, pn)
				return false, nil
			}
			return true, nil
		})

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

type PHPLanguageSupportDefinition struct{}

func NewPHPLanguageSupportDefinition() *PHPLanguageSupportDefinition {
	return &PHPLanguageSupportDefinition{}
}

var (
	stubsRE  = regexp.MustCompile(`(?i)/intelephense/lib/stub/`)
	vendorRE = regexp.MustCompile(`(?i)(^|/)vendor/`)
)

func (plsd *PHPLanguageSupportDefinition) ClassifyImportType(s *common.Source) common.DependencyType {
	p := filepath.ToSlash(filepath.Clean(s.Path))

	if stubsRE.MatchString(p) {
		return common.StandardLibraryDependency
	}

	if vendorRE.MatchString(p) {
		return common.PackageDependency
	}

	return common.LocalDependency
}

func (plsd *PHPLanguageSupportDefinition) GetLSPServerRequirement() lsp.ServerRequirement {
	// banstructlit:ignore
	return lsp.ServerRequirement{
		Name:           intelephenseName,
		InstallCommand: intelephenseInstallCommand,
		Locate:         intelephensePath,
	}
}

func (plsd *PHPLanguageSupportDefinition) NewLSPServer(ctx context.Context) (*lsp.ServerProcess, error) {
	execPath, err := intelephensePath(ctx)

	if err != nil {
		return nil, err
	}

	// intelephense only speaks LSP over stdio when asked; without the flag
	// it hangs at initialize
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

func (plsd *PHPLanguageSupportDefinition) Parse(ctx context.Context, src *common.Source, tree *tree_sitter.Tree) (*ast.ASTModule, error) {
	name := strings.TrimSuffix(filepath.Base(src.Path), filepath.Ext(src.Path))

	root := ast.NewASTModule(ast.NewASTNodeContainer(
		parsers.NewParserNode(src, tree.RootNode()),
	), name)

	if err := parsers.WalkTransformTree(ctx, src, tree, root, PHPTransformer); err != nil {
		return nil, err
	}

	return root, nil
}

func (plsd *PHPLanguageSupportDefinition) GetLanguageID() string {
	return "php"
}

func (plsd *PHPLanguageSupportDefinition) GetTreeSitterLanguage() *tree_sitter.Language {
	return tree_sitter.NewLanguage(tree_sitter_php.LanguagePHP())
}
