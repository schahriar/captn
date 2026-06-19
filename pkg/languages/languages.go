package languages

import (
	"context"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/common"
	languages_golang "github.com/schahriar/captn/pkg/languages/golang"
	"github.com/schahriar/captn/pkg/lsp"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

type LanguageSupport interface {
	// TODO: Add LSP installer
	Parse(ctx context.Context, src *common.Source, tree *tree_sitter.Tree) (*ast.ASTModule, error)
	NewLSPServer(ctx context.Context) (*lsp.ServerProcess, error)
	GetLanguageID() string
	ClassifyImportType(*common.Source) common.DependencyType
	GetTreeSitterLanguage() *tree_sitter.Language
}

var Golang LanguageSupport = &languages_golang.GolangLanguageSupportDefinition{}
