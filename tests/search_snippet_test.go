package tests_test

import (
	"os"
	"testing"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/stretchr/testify/assert"
)

func TestSearchSnippetRootsAtEnclosingFunction(t *testing.T) {
	cwd, err := os.Getwd()
	assert.NoError(t, err)

	wspace := cog.NewWorkspace(cwd)

	og, root, err := wspace.SearchSnippet(t.Context(), "./fixtures/golang/multidep/cmd/main.go", "fmt.Println(fixture_dep1.GetExampleText())")

	if !assert.NoError(t, err) {
		return
	}

	_, ok := root.(*ast.ASTFuncExpression)
	assert.True(t, ok, "expected root to be the enclosing function, got %T", root)

	f, err := wspace.LoadFile(t.Context(), "./fixtures/golang/multidep/cmd/main.go")
	assert.NoError(t, err)

	adj, err := og.Graph.AdjacencyMap()
	assert.NoError(t, err)
	assert.NotContains(t, adj, f.Module.GetHash())
}
