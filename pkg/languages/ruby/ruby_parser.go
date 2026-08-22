package languages_ruby

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
	tree_sitter_ruby "github.com/tree-sitter/tree-sitter-ruby/bindings/go"
)

// hasSourceWidth reports whether a node spans any bytes; tree-sitter error
// recovery produces zero-width nodes that must never become AST nodes (the
// interval index rejects empty ranges)
func hasSourceWidth(node parsers.ParserNode) bool {
	return node.Range.StartByte < node.Range.EndByte
}

// typeExpression maps a constant reference (Ruby's only type spelling) onto
// the identifier a definition resolves to
func typeExpression(node parsers.ParserNode) *ast.ASTTypeExpression {
	switch node.Kind {
	case "constant":
		if !hasSourceWidth(node) {
			return nil
		}

		return ast.NewASTTypeExpression(ast.NewASTNodeContainer(node), node.GetTextContent())

	case "scope_resolution":
		nameNode, ok := node.ChildByFieldName("name")

		if !ok || !hasSourceWidth(nameNode) {
			return nil
		}

		texpr := ast.NewASTTypeExpression(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())

		if scopeNode, ok := node.ChildByFieldName("scope"); ok && scopeNode.Kind == "constant" {
			texpr.Namespace = ast.NewASTSymbol(ast.NewASTNodeContainer(scopeNode), scopeNode.GetTextContent())
		}

		return texpr
	}

	return nil
}

// collectBindingTargets collects the identifiers an assignment target binds.
// LSP definitions land on these, so every bound name needs a symbol;
// index and attribute targets declare nothing nameable.
func collectBindingTargets(node parsers.ParserNode) []*ast.ASTSymbol {
	switch node.Kind {
	case "identifier", "constant", "instance_variable", "class_variable", "global_variable":
		if !hasSourceWidth(node) {
			return nil
		}
		return []*ast.ASTSymbol{ast.NewASTSymbol(ast.NewASTNodeContainer(node), node.GetTextContent())}

	case "left_assignment_list", "destructured_left_assignment", "rest_assignment":
		var names []*ast.ASTSymbol
		node.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
			names = append(names, collectBindingTargets(pn)...)
			return true, nil
		})
		return names
	}

	return nil
}

func appendParameters(fn *ast.ASTFuncExpression, list parsers.ParserNode) {
	list.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
		switch pn.Kind {
		case "identifier":
			if hasSourceWidth(pn) {
				sym := ast.NewASTSymbol(ast.NewASTNodeContainer(pn), pn.GetTextContent())
				fn.Arguments = append(fn.Arguments, ast.NewASTFuncArgument(ast.NewASTNodeContainer(pn), sym, nil))
			}

		case "optional_parameter", "keyword_parameter", "splat_parameter", "hash_splat_parameter", "block_parameter":
			if nameNode, ok := pn.ChildByFieldName("name"); ok && hasSourceWidth(nameNode) {
				sym := ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
				fn.Arguments = append(fn.Arguments, ast.NewASTFuncArgument(ast.NewASTNodeContainer(pn), sym, nil))
			}

		case "destructured_parameter":
			appendParameters(fn, pn)
		}

		return true, nil
	})
}

// emitFuncExpression emits a FuncExpression for a def, class, module or
// lambda node and narrows its block to the body so the function stays
// visible to range queries. An empty body keeps the shadow deliberately.
func emitFuncExpression(ctx context.Context, trx *parsers.TransformContext, node parsers.ParserNode, fn *ast.ASTFuncExpression) error {
	if bodyNode, ok := node.ChildByFieldName("body"); ok && hasSourceWidth(bodyNode) {
		fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(bodyNode))
	}

	if err := trx.Emit(fn); err != nil {
		return err
	}

	return trx.WalkChildrenInto(ctx, fn)
}

// requirePath finds the string content of a require/require_relative call
// argument; interpolated or computed paths have none
func requirePath(argsNode parsers.ParserNode) (parsers.ParserNode, bool) {
	strNode, ok := argsNode.GetNthChildByKind("string", 0)

	if !ok {
		return strNode, false
	}

	contentNode, ok := strNode.GetNthChildByKind("string_content", 0)

	if !ok || !hasSourceWidth(contentNode) {
		var zero parsers.ParserNode
		return zero, false
	}

	return contentNode, true
}

// attrNames collects the simple_symbol arguments of an attr_accessor-family
// call. Each one is where ruby-lsp resolves the defined reader/writer, so
// each needs a symbol node
func attrNames(argsNode parsers.ParserNode) []*ast.ASTSymbol {
	var names []*ast.ASTSymbol

	argsNode.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
		if pn.Kind == "simple_symbol" && hasSourceWidth(pn) {
			name := strings.TrimPrefix(pn.GetTextContent(), ":")
			names = append(names, ast.NewASTSymbol(ast.NewASTNodeContainer(pn), name))
		}
		return true, nil
	})

	return names
}

func RubyTransformer(ctx context.Context, trx *parsers.TransformContext, node parsers.ParserNode) error {
	switch node.Kind {
	case "class", "module":
		// The anonymous "class"/"module" keyword tokens share these Kinds;
		// only the declaration node carries a name field
		nameNode, ok := node.ChildByFieldName("name")
		if !ok {
			return trx.WalkChildren(ctx)
		}

		// Classes and modules map to FuncExpression on purpose: LSP
		// definitions of constant references land on their name, and each
		// becomes an observation vertex like any function
		fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))

		// A nested name (class Foo::Bar) declares the trailing constant
		if nameNode.Kind == "scope_resolution" {
			if inner, ok := nameNode.ChildByFieldName("name"); ok {
				nameNode = inner
			}
		}

		if hasSourceWidth(nameNode) {
			fn.Name = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
		}

		if bodyNode, ok := node.ChildByFieldName("body"); ok && hasSourceWidth(bodyNode) {
			fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(bodyNode))
		}

		if superNode, ok := node.ChildByFieldName("superclass"); ok {
			superNode.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
				if texpr := typeExpression(pn); texpr != nil {
					fn.AppendChild(texpr)
					return false, nil
				}
				return true, nil
			})
		}

		if err := trx.Emit(fn); err != nil {
			return err
		}

		return trx.WalkChildrenInto(ctx, fn)

	case "method", "singleton_method":
		fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))

		if nameNode, ok := node.ChildByFieldName("name"); ok && hasSourceWidth(nameNode) {
			fn.Name = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
		}

		if paramsNode, ok := node.ChildByFieldName("parameters"); ok {
			appendParameters(fn, paramsNode)
		}

		return emitFuncExpression(ctx, trx, node, fn)

	case "lambda":
		fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))

		if paramsNode, ok := node.ChildByFieldName("parameters"); ok {
			appendParameters(fn, paramsNode)
		}

		return emitFuncExpression(ctx, trx, node, fn)

	case "assignment", "operator_assignment":
		var names []*ast.ASTSymbol

		if leftNode, ok := node.ChildByFieldName("left"); ok {
			names = collectBindingTargets(leftNode)
		}

		if len(names) == 0 {
			return trx.WalkChildren(ctx)
		}

		decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(node))
		decl.Names = names

		if err := trx.Emit(decl); err != nil {
			return err
		}

		// RHS: walk into decl so call expressions on the right-hand side
		// land in Virtual
		return trx.WalkChildrenInto(ctx, decl)

	case "call":
		methodNode, hasMethod := node.ChildByFieldName("method")
		_, hasReceiver := node.ChildByFieldName("receiver")

		// Ruby has no import syntax; require is a plain method call, and
		// attr_accessor-family calls define the methods reader definitions
		// resolve onto
		if hasMethod && !hasReceiver && methodNode.Kind == "identifier" {
			if argsNode, ok := node.ChildByFieldName("arguments"); ok {
				switch methodNode.GetTextContent() {
				case "require", "require_relative":
					if pathNode, ok := requirePath(argsNode); ok {
						imp := ast.NewASTImportStatement(ast.NewASTNodeContainer(pathNode))
						imp.Reference = ast.NewASTSymbol(ast.NewASTNodeContainer(pathNode), pathNode.GetTextContent())
						return trx.Emit(imp)
					}

				case "attr_accessor", "attr_reader", "attr_writer":
					if names := attrNames(argsNode); len(names) > 0 {
						decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(node))
						decl.Names = names
						return trx.Emit(decl)
					}
				}
			}
		}

		callExpr := ast.NewASTCallExpression(ast.NewASTNodeContainer(node))

		if hasMethod && hasSourceWidth(methodNode) {
			switch methodNode.Kind {
			case "identifier", "constant":
				callExpr.Symbol = ast.NewASTSymbol(ast.NewASTNodeContainer(methodNode), methodNode.GetTextContent())
			}
		}

		if recvNode, ok := node.ChildByFieldName("receiver"); ok {
			switch recvNode.Kind {
			case "identifier", "constant":
				callExpr.Namespace = ast.NewASTSymbol(ast.NewASTNodeContainer(recvNode), recvNode.GetTextContent())
			}
		}

		// No identifiable callee (operator or dynamic call); its children
		// still index under the enclosing block
		if callExpr.Symbol == nil {
			return trx.WalkChildren(ctx)
		}

		if err := trx.Emit(callExpr); err != nil {
			return err
		}

		// Handles chained receivers, argument calls and attached blocks
		return trx.WalkChildrenInto(ctx, callExpr)

	default:
		return trx.WalkChildren(ctx)
	}
}

type RubyLanguageSupportDefinition struct{}

func NewRubyLanguageSupportDefinition() *RubyLanguageSupportDefinition {
	return &RubyLanguageSupportDefinition{}
}

var (
	gemsRE = regexp.MustCompile(`/gems/[^/]+/`)
	// Interpreter stdlib sits under lib/ruby/<version> across ruby.org,
	// Homebrew, rbenv, rvm and system layouts; gem paths also contain
	// lib/ruby but always with /gems/ after it, so gems are ruled out first
	rubyLibRE = regexp.MustCompile(`(?i)/lib/ruby/\d+(\.\d+)*/`)
)

func (rlsd *RubyLanguageSupportDefinition) ClassifyImportType(s *common.Source) common.DependencyType {
	p := filepath.ToSlash(filepath.Clean(s.Path))

	if gemsRE.MatchString(p) {
		return common.PackageDependency
	}

	if rubyLibRE.MatchString(p) {
		return common.StandardLibraryDependency
	}

	return common.LocalDependency
}

func (rlsd *RubyLanguageSupportDefinition) GetLSPServerRequirement() lsp.ServerRequirement {
	// banstructlit:ignore
	return lsp.ServerRequirement{
		Name:           rubyLSPName,
		InstallCommand: rubyLSPInstallCommand,
		Locate:         rubyLSPPath,
	}
}

func (rlsd *RubyLanguageSupportDefinition) NewLSPServer(ctx context.Context) (*lsp.ServerProcess, error) {
	execPath, err := rubyLSPPath(ctx)

	if err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, execPath)
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

func (rlsd *RubyLanguageSupportDefinition) Parse(ctx context.Context, src *common.Source, tree *tree_sitter.Tree) (*ast.ASTModule, error) {
	name := strings.TrimSuffix(filepath.Base(src.Path), filepath.Ext(src.Path))

	root := ast.NewASTModule(ast.NewASTNodeContainer(
		parsers.NewParserNode(src, tree.RootNode()),
	), name)

	if err := parsers.WalkTransformTree(ctx, src, tree, root, RubyTransformer); err != nil {
		return nil, err
	}

	return root, nil
}

func (rlsd *RubyLanguageSupportDefinition) GetLanguageID() string {
	return "ruby"
}

func (rlsd *RubyLanguageSupportDefinition) GetTreeSitterLanguage() *tree_sitter.Language {
	return tree_sitter.NewLanguage(tree_sitter_ruby.Language())
}
