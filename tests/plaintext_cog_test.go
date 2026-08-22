package tests_test

import (
	"os"
	"testing"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/stretchr/testify/assert"
)

func TestPlaintextParsesWholeFileModule(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/plaintext/notes.md")

	assert.Equal(t, "notes.md", pf.Module.Name)
	assert.Empty(t, pf.Module.Block.Children())
	assert.Equal(t, string(pf.Source.Buffer), pf.Module.GetStringSource())
	assert.Equal(t, "plaintext", pf.Module.GetLanguage())
	assert.Same(t, pf.Module, pf.Module.Block.GetParent())
}

func TestPlaintextHashIsStable(t *testing.T) {
	a := parseTestFile(t, "./fixtures/plaintext/notes.md")
	b := parseTestFile(t, "./fixtures/plaintext/notes.md")

	assert.Equal(t, ast.GetHash(a.Module), ast.GetHash(b.Module))
}

func TestPlaintextEmptyFileParses(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/plaintext/empty.txt")

	assert.Empty(t, pf.Module.GetStringSource())
}

func TestPlaintextListDependenciesNeedsNoServer(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/plaintext/notes.md")

	imps, err := pf.ListDependencies(t.Context())

	assert.NoError(t, err)
	assert.Empty(t, imps)
}

func TestPlaintextSearchSnippetRootsAtModule(t *testing.T) {
	cwd, err := os.Getwd()
	assert.NoError(t, err)

	c := cog.NewWorkspace(cwd)

	og, root, err := c.SearchSnippet(t.Context(), "./fixtures/plaintext/notes.md", "rotate the demo API key")
	if !assert.NoError(t, err) {
		return
	}

	pf, err := c.LoadFile(t.Context(), "./fixtures/plaintext/notes.md")
	if !assert.NoError(t, err) {
		return
	}

	assert.Same(t, pf.Module, root)

	adj, err := og.Graph.AdjacencyMap()
	assert.NoError(t, err)
	assert.Len(t, adj, 1)
}

func TestPlaintextModuleSnap(t *testing.T) {
	checkSnapshot(t, "./fixtures/plaintext/notes.md")
}
