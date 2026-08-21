package languages

import (
	"context"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/common"
	languages_golang "github.com/schahriar/captn/pkg/languages/golang"
	languages_python "github.com/schahriar/captn/pkg/languages/python"
	languages_swift "github.com/schahriar/captn/pkg/languages/swift"
	languages_typescript "github.com/schahriar/captn/pkg/languages/typescript"
	"github.com/schahriar/captn/pkg/lsp"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

type LanguageSupport interface {
	Parse(ctx context.Context, src *common.Source, tree *tree_sitter.Tree) (*ast.ASTModule, error)
	NewLSPServer(ctx context.Context) (*lsp.ServerProcess, error)
	GetLSPServerRequirement() lsp.ServerRequirement
	GetLSPInitializationOptions(ctx context.Context, workspace string) any
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

// TypeScript's sibling dialects share one transformer and one server but
// carry their own grammar and didOpen languageId
var Typescript LanguageSupport = languages_typescript.NewTypescriptLanguageSupportDefinition()
var TypescriptReact LanguageSupport = languages_typescript.NewTSXLanguageSupportDefinition()
var Javascript LanguageSupport = languages_typescript.NewJavascriptLanguageSupportDefinition()
var JavascriptReact LanguageSupport = languages_typescript.NewJSXLanguageSupportDefinition()

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
	case ".ts", ".mts", ".cts":
		return Typescript, true
	case ".tsx":
		return TypescriptReact, true
	case ".js", ".mjs", ".cjs":
		return Javascript, true
	case ".jsx":
		return JavascriptReact, true
	}

	return nil, false
}
