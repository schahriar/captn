package languages_plaintext

import (
	"context"
	"path/filepath"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/knownerr"
	"github.com/schahriar/captn/pkg/lsp"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// wholeFileNode implements ast.ASTParserNode without a grammar tree behind it
type wholeFileNode struct {
	src *common.Source
	rng *common.FileRange
}

func newWholeFileNode(src *common.Source) wholeFileNode {
	return wholeFileNode{
		src: src,
		rng: common.NewFileRange(src, common.FirstPositionOfSource(src), common.LastPositionOfSource(src)),
	}
}

func (n wholeFileNode) GetSource() *common.Source {
	return n.src
}

func (n wholeFileNode) GetPosition() *common.FileRange {
	return n.rng
}

type PlaintextLanguageSupportDefinition struct{}

func NewPlaintextLanguageSupportDefinition() *PlaintextLanguageSupportDefinition {
	return &PlaintextLanguageSupportDefinition{}
}

// tree is always nil here: plaintext has no grammar
func (plsd *PlaintextLanguageSupportDefinition) Parse(ctx context.Context, src *common.Source, tree *tree_sitter.Tree) (*ast.ASTModule, error) {
	root := ast.NewASTModule(ast.NewASTNodeContainer(newWholeFileNode(src)), filepath.Base(src.Path))

	ast.AttachParents(root)

	return root, nil
}

func (plsd *PlaintextLanguageSupportDefinition) NewLSPServer(ctx context.Context) (*lsp.ServerProcess, error) {
	return nil, knownerr.UnsupportedFeature("plaintext files have no language server")
}

func (plsd *PlaintextLanguageSupportDefinition) GetLSPServerRequirement() lsp.ServerRequirement {
	// banstructlit:ignore
	return lsp.ServerRequirement{}
}

func (plsd *PlaintextLanguageSupportDefinition) GetLSPInitializationOptions(ctx context.Context, workspace string) any {
	return nil
}

func (plsd *PlaintextLanguageSupportDefinition) GetLanguageID() string {
	return "plaintext"
}

func (plsd *PlaintextLanguageSupportDefinition) ClassifyImportType(s *common.Source) common.DependencyType {
	return common.LocalDependency
}

func (plsd *PlaintextLanguageSupportDefinition) NormalizeDefinitionRange(src *common.Source, r *common.FileRange) *common.FileRange {
	return r
}

func (plsd *PlaintextLanguageSupportDefinition) GetTreeSitterLanguage() *tree_sitter.Language {
	return nil
}
