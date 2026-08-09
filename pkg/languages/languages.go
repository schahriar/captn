package languages

import (
	"context"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/common"
	languages_golang "github.com/schahriar/captn/pkg/languages/golang"
	languages_python "github.com/schahriar/captn/pkg/languages/python"
	"github.com/schahriar/captn/pkg/lsp"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

type LanguageSupport interface {
	Parse(ctx context.Context, src *common.Source, tree *tree_sitter.Tree) (*ast.ASTModule, error)
	NewLSPServer(ctx context.Context) (*lsp.ServerProcess, error)
	GetLSPServerRequirement() lsp.ServerRequirement
	GetLanguageID() string
	ClassifyImportType(*common.Source) common.DependencyType
	GetTreeSitterLanguage() *tree_sitter.Language
}

var Golang LanguageSupport = languages_golang.NewGolangLanguageSupportDefinition()
var Python LanguageSupport = languages_python.NewPythonLanguageSupportDefinition()

// ForExtension resolves the LanguageSupport that parses files with the given
// extension, e.g. ".go"
func ForExtension(ext string) (LanguageSupport, bool) {
	switch ext {
	case ".go":
		return Golang, true
	case ".py", ".pyi":
		return Python, true
	}

	return nil, false
}
