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
	NewLSPServer(ctx context.Context) (*lsp.ServerProcess, error)
	Parse(ctx context.Context, src *common.Source, tree *tree_sitter.Tree) (*ast.ASTModule, error)
}

var Golang LanguageSupport = &languages_golang.GolangLanguageSupportDefinition{}
