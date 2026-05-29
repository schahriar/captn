package languages

import (
	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/common"
	languages_golang "github.com/schahriar/captn/pkg/languages/golang"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

type LanguageSupport interface {
	Parse(src *common.Source, tree *tree_sitter.Tree) (*ast.ASTModule, error)
}

var Golang LanguageSupport = &languages_golang.GolangLanguageSupportDefinition{}
