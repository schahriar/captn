package languages_html

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
	tree_sitter_html "github.com/tree-sitter/tree-sitter-html/bindings/go"
)

// hasSourceWidth reports whether a node spans any bytes; tree-sitter error
// recovery produces zero-width nodes that must never become AST nodes (the
// interval index rejects empty ranges)
func hasSourceWidth(node parsers.ParserNode) bool {
	return node.Range.StartByte < node.Range.EndByte
}

// openingTag answers the tag an element is named and attributed by: the
// start tag for a paired element, the tag itself for a self-closing one
func openingTag(node parsers.ParserNode) (parsers.ParserNode, bool) {
	if tag, ok := node.GetNthChildByKind("start_tag", 0); ok {
		return tag, true
	}

	return node.GetNthChildByKind("self_closing_tag", 0)
}

// A value-less attribute is nothing but its name, so its argument and
// identifier share one range and the symbol wins it, the same shape the
// reference accepts for a bare type parameter: no wider node exists to
// anchor the argument on
func appendAttributes(fn *ast.ASTFuncExpression, tag parsers.ParserNode) {
	tag.IterateChildren(func(attr parsers.ParserNode) (bool, error) {
		if attr.Kind != "attribute" {
			return true, nil
		}

		var sym *ast.ASTSymbol

		if nameNode, ok := attr.GetNthChildByKind("attribute_name", 0); ok && hasSourceWidth(nameNode) {
			sym = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
		}

		fn.Arguments = append(fn.Arguments, ast.NewASTFuncArgument(ast.NewASTNodeContainer(attr), sym, nil))

		return true, nil
	})
}

// HTML maps no imports: <link href> and <script src> are its include forms,
// but the html server resolves no definitions, so Import nodes would only
// generate dead requests. Revisit if the server ever answers them.
func HTMLTransformer(ctx context.Context, trx *parsers.TransformContext, node parsers.ParserNode) error {
	switch node.Kind {
	case "element", "script_element", "style_element":
		fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))

		// An element has no single body node between its tags. The block
		// narrows onto the opening tag instead: left on the element's own
		// span it would shadow every element in the interval index and all
		// observations would root at the file. A self-closing or void
		// element flush against the next byte IS its opening tag, so it
		// keeps the shadow deliberately and a snippet on it roots at the
		// enclosing element; trailing whitespace widens the element node
		// and lifts the shadow, which is fine either way.
		if tag, ok := openingTag(node); ok {
			if nameNode, ok := tag.GetNthChildByKind("tag_name", 0); ok && hasSourceWidth(nameNode) {
				fn.Name = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
			}

			appendAttributes(fn, tag)

			fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(tag))
		}

		if err := trx.Emit(fn); err != nil {
			return err
		}

		return trx.WalkChildrenInto(ctx, fn)

	case "start_tag", "self_closing_tag":
		// The element case already read the tag; walking into it would find
		// nothing but the attributes it consumed
		return nil

	case "comment":
		return nil

	default:
		return trx.WalkChildren(ctx)
	}
}

type HTMLLanguageSupportDefinition struct{}

func NewHTMLLanguageSupportDefinition() *HTMLLanguageSupportDefinition {
	return &HTMLLanguageSupportDefinition{}
}

var htmlNodeModulesRE = regexp.MustCompile(`(?i)(^|/)node_modules/`)

// HTML has no standard library to resolve into; everything is either the
// workspace's own or an installed package
func (hlsd *HTMLLanguageSupportDefinition) ClassifyImportType(s *common.Source) common.DependencyType {
	p := s.Path

	if !filepath.IsAbs(p) && s.Workspace != "" {
		p = filepath.Join(s.Workspace, p)
	}

	if htmlNodeModulesRE.MatchString(filepath.ToSlash(filepath.Clean(p))) {
		return common.PackageDependency
	}

	return common.LocalDependency
}

func (hlsd *HTMLLanguageSupportDefinition) GetLSPServerRequirement() lsp.ServerRequirement {
	// banstructlit:ignore
	return lsp.ServerRequirement{
		Name:           htmlServerName,
		InstallCommand: vscodeServersInstallCommand,
		Locate:         htmlServerPath,
	}
}

func (hlsd *HTMLLanguageSupportDefinition) NewLSPServer(ctx context.Context) (*lsp.ServerProcess, error) {
	execPath, err := htmlServerPath(ctx)

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

func (hlsd *HTMLLanguageSupportDefinition) GetLSPInitializationOptions(_ context.Context, _ string) any {
	return nil
}

func (hlsd *HTMLLanguageSupportDefinition) Parse(ctx context.Context, src *common.Source, tree *tree_sitter.Tree) (*ast.ASTModule, error) {
	name := strings.TrimSuffix(filepath.Base(src.Path), filepath.Ext(src.Path))

	root := ast.NewASTModule(ast.NewASTNodeContainer(
		parsers.NewParserNode(src, tree.RootNode()),
	), name)

	if err := parsers.WalkTransformTree(ctx, src, tree, root, HTMLTransformer); err != nil {
		return nil, err
	}

	return root, nil
}

func (hlsd *HTMLLanguageSupportDefinition) GetLanguageID() string {
	return "html"
}

func (hlsd *HTMLLanguageSupportDefinition) GetTreeSitterLanguage() *tree_sitter.Language {
	return tree_sitter.NewLanguage(tree_sitter_html.Language())
}
