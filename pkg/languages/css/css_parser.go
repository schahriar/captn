package languages_css

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
	tree_sitter_css "github.com/tree-sitter/tree-sitter-css/bindings/go"
)

// hasSourceWidth reports whether a node spans any bytes; tree-sitter error
// recovery produces zero-width nodes that must never become AST nodes (the
// interval index rejects empty ranges)
func hasSourceWidth(node parsers.ParserNode) bool {
	return node.Range.StartByte < node.Range.EndByte
}

// firstNameOfKind walks a selector subtree for the first node of the given
// kind, depth-first in document order
func firstNameOfKind(node parsers.ParserNode, kind string) (parsers.ParserNode, bool) {
	if node.Kind == kind && hasSourceWidth(node) {
		return node, true
	}

	var found parsers.ParserNode
	ok := false

	node.IterateChildren(func(child parsers.ParserNode) (bool, error) {
		if inner, innerOk := firstNameOfKind(child, kind); innerOk {
			found = inner
			ok = true
			return false, nil
		}
		return true, nil
	})

	return found, ok
}

// firstSelectorName is firstNameOfKind restricted to real selector names.
// The grammar wraps a compound selector inside its pseudo-class, spelling
// the pseudo's own name with the class_name kind too (`.card:hover` holds
// class_selector(.card) and class_name "hover"), so a pseudo's direct name
// and its arguments are skipped: naming .card:hover "hover" or div:not(.a)
// "a" would give the vertex a misleading identity.
func firstSelectorName(node parsers.ParserNode, kind string) (parsers.ParserNode, bool) {
	if node.Kind == kind && hasSourceWidth(node) {
		return node, true
	}

	pseudo := node.Kind == "pseudo_class_selector" || node.Kind == "pseudo_element_selector"

	var found parsers.ParserNode
	ok := false

	node.IterateChildren(func(child parsers.ParserNode) (bool, error) {
		if pseudo && (child.Kind == kind || child.Kind == "arguments") {
			return true, nil
		}

		if inner, innerOk := firstSelectorName(child, kind); innerOk {
			found = inner
			ok = true
			return false, nil
		}
		return true, nil
	})

	return found, ok
}

// A rule is named for the first class in its selectors, falling back to an
// id; a bare tag or pseudo-only rule stays anonymous
func ruleName(selectors parsers.ParserNode) *ast.ASTSymbol {
	for _, kind := range []string{"class_name", "id_name"} {
		if nameNode, ok := firstSelectorName(selectors, kind); ok {
			return ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
		}
	}

	return nil
}

func firstStringValue(node parsers.ParserNode) (parsers.ParserNode, bool) {
	return firstNameOfKind(node, "string_value")
}

func isVariableName(name string) bool {
	return strings.HasPrefix(name, "--")
}

func CSSTransformer(ctx context.Context, trx *parsers.TransformContext, node parsers.ParserNode) error {
	switch node.Kind {
	case "rule_set":
		fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))

		if selectors, ok := node.GetNthChildByKind("selectors", 0); ok {
			fn.Name = ruleName(selectors)
		}

		if blockNode, ok := node.GetNthChildByKind("block", 0); ok {
			fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(blockNode))
		}

		if err := trx.Emit(fn); err != nil {
			return err
		}

		return trx.WalkChildrenInto(ctx, fn)

	case "declaration":
		// A custom property declaration is where a variable's definition
		// resolves to; the walk continues so var() uses in its value are
		// still mapped
		if propNode, ok := node.GetNthChildByKind("property_name", 0); ok {
			if hasSourceWidth(propNode) && isVariableName(propNode.GetTextContent()) {
				if err := trx.Emit(ast.NewASTSymbol(ast.NewASTNodeContainer(propNode), propNode.GetTextContent())); err != nil {
					return err
				}
			}
		}

		return trx.WalkChildren(ctx)

	case "call_expression":
		// Only var() carries a name a definition can resolve; other value
		// functions fall through so any var() nested in them is still
		// found. CSS function names are case-insensitive.
		fnNode, ok := node.GetNthChildByKind("function_name", 0)

		if !ok || !strings.EqualFold(fnNode.GetTextContent(), "var") {
			return trx.WalkChildren(ctx)
		}

		callExpr := ast.NewASTCallExpression(ast.NewASTNodeContainer(node))

		if argsNode, ok := node.GetNthChildByKind("arguments", 0); ok {
			argsNode.IterateChildren(func(arg parsers.ParserNode) (bool, error) {
				if arg.Kind == "plain_value" && hasSourceWidth(arg) && isVariableName(arg.GetTextContent()) {
					callExpr.Symbol = ast.NewASTSymbol(ast.NewASTNodeContainer(arg), arg.GetTextContent())
					return false, nil
				}
				return true, nil
			})
		}

		if callExpr.Symbol == nil {
			return trx.WalkChildren(ctx)
		}

		if err := trx.Emit(callExpr); err != nil {
			return err
		}

		// A fallback value can hold another var()
		return trx.WalkChildrenInto(ctx, callExpr)

	case "import_statement":
		if stringNode, ok := firstStringValue(node); ok {
			impExpr := ast.NewASTImportStatement(ast.NewASTNodeContainer(stringNode))

			if content, ok := stringNode.GetNthChildByKind("string_content", 0); ok && hasSourceWidth(content) {
				impExpr.Reference = ast.NewASTSymbol(ast.NewASTNodeContainer(stringNode), content.GetTextContent())
			}

			if impExpr.Reference == nil {
				return nil
			}

			return trx.Emit(impExpr)
		}

		// An unquoted url(x.css) keeps its path as a plain value
		if pathNode, ok := firstNameOfKind(node, "plain_value"); ok {
			impExpr := ast.NewASTImportStatement(ast.NewASTNodeContainer(pathNode))
			impExpr.Reference = ast.NewASTSymbol(ast.NewASTNodeContainer(pathNode), pathNode.GetTextContent())

			return trx.Emit(impExpr)
		}

		return nil

	case "comment":
		return nil

	default:
		return trx.WalkChildren(ctx)
	}
}

// CSSLanguageSupportDefinition also covers SCSS and LESS as degraded
// dialects: no maintained grammar with a Go binding exists for either, and
// under the CSS grammar their rule, class and nesting structure still parses
// while $ and @ variables fall into ERROR nodes that stay as unmapped bytes
// inside the enclosing rule. The server understands both dialects natively,
// so the didOpen languageId is what varies.
type CSSLanguageSupportDefinition struct {
	languageID string
}

func NewCSSLanguageSupportDefinition() *CSSLanguageSupportDefinition {
	return &CSSLanguageSupportDefinition{languageID: "css"}
}

func NewSCSSLanguageSupportDefinition() *CSSLanguageSupportDefinition {
	clsd := NewCSSLanguageSupportDefinition()
	clsd.languageID = "scss"
	return clsd
}

func NewLESSLanguageSupportDefinition() *CSSLanguageSupportDefinition {
	clsd := NewCSSLanguageSupportDefinition()
	clsd.languageID = "less"
	return clsd
}

var cssNodeModulesRE = regexp.MustCompile(`(?i)(^|/)node_modules/`)

// CSS has no standard library to resolve into; everything is either the
// workspace's own or an installed package. Today's server only answers
// definitions inside the opened document, so cross-file paths are
// future-proofing, not probed behavior.
func (clsd *CSSLanguageSupportDefinition) ClassifyImportType(s *common.Source) common.DependencyType {
	p := s.Path

	if !filepath.IsAbs(p) && s.Workspace != "" {
		p = filepath.Join(s.Workspace, p)
	}

	if cssNodeModulesRE.MatchString(filepath.ToSlash(filepath.Clean(p))) {
		return common.PackageDependency
	}

	return common.LocalDependency
}

func (clsd *CSSLanguageSupportDefinition) GetLSPServerRequirement() lsp.ServerRequirement {
	// banstructlit:ignore
	return lsp.ServerRequirement{
		Name:           cssServerName,
		InstallCommand: vscodeServersInstallCommand,
		Locate:         cssServerPath,
	}
}

func (clsd *CSSLanguageSupportDefinition) NewLSPServer(ctx context.Context) (*lsp.ServerProcess, error) {
	execPath, err := cssServerPath(ctx)

	if err != nil {
		return nil, err
	}

	// The vscode language servers only speak LSP over stdio when asked;
	// without the flag they hang at initialize
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

func (clsd *CSSLanguageSupportDefinition) GetLSPInitializationOptions(_ context.Context, _ string) any {
	return nil
}

func (clsd *CSSLanguageSupportDefinition) Parse(ctx context.Context, src *common.Source, tree *tree_sitter.Tree) (*ast.ASTModule, error) {
	name := strings.TrimSuffix(filepath.Base(src.Path), filepath.Ext(src.Path))

	root := ast.NewASTModule(ast.NewASTNodeContainer(
		parsers.NewParserNode(src, tree.RootNode()),
	), name)

	if err := parsers.WalkTransformTree(ctx, src, tree, root, CSSTransformer); err != nil {
		return nil, err
	}

	return root, nil
}

func (clsd *CSSLanguageSupportDefinition) GetLanguageID() string {
	return clsd.languageID
}

func (clsd *CSSLanguageSupportDefinition) GetTreeSitterLanguage() *tree_sitter.Language {
	return tree_sitter.NewLanguage(tree_sitter_css.Language())
}
