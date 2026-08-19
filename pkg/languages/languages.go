package languages

import (
	"context"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/common"
	languages_golang "github.com/schahriar/captn/pkg/languages/golang"
	languages_python "github.com/schahriar/captn/pkg/languages/python"
	languages_swift "github.com/schahriar/captn/pkg/languages/swift"
	"github.com/schahriar/captn/pkg/lsp"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

type LanguageSupport interface {
	Parse(ctx context.Context, src *common.Source, tree *tree_sitter.Tree) (*ast.ASTModule, error)
	NewLSPServer(ctx context.Context) (*lsp.ServerProcess, error)
	GetLSPServerRequirement() lsp.ServerRequirement
	GetLanguageID() string
	ClassifyImportType(*common.Source) common.DependencyType
	// NormalizeDefinitionRange repairs the range shape a language server
	// answers textDocument/definition with, before it is resolved to a node.
	// Servers disagree: most span the identifier.
	// Mainly added for Swift's sourcekit-lsp support which sends a zero-width
	// position at its start.
	NormalizeDefinitionRange(*common.Source, *common.FileRange) *common.FileRange
	GetTreeSitterLanguage() *tree_sitter.Language
}

var Golang LanguageSupport = languages_golang.NewGolangLanguageSupportDefinition()
var Python LanguageSupport = languages_python.NewPythonLanguageSupportDefinition()
var Swift LanguageSupport = languages_swift.NewSwiftLanguageSupportDefinition()

// ForExtension resolves the LanguageSupport that parses files with the given
// extension, e.g. ".go"
func ForExtension(ext string) (LanguageSupport, bool) {
	switch ext {
	case ".go":
		return Golang, true
	case ".py", ".pyi":
		return Python, true
	// Every Swift definition that leaves the current module resolves into a
	// generated .swiftinterface; leaving it undispatched drops the whole match
	case ".swift", ".swiftinterface":
		return Swift, true
	}

	return nil, false
}
