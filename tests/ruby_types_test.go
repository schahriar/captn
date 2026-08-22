package tests_test

import (
	"testing"
	"time"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/stretchr/testify/assert"
)

// Ruby spells types only as constant references (superclasses); each
// TypeExpression must span its identifier alone so a definition resolves
// onto exactly one node
func TestRubyTypeExpressionsSpanIdentifiers(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/ruby/baseproj/types.rb")

	var types []*ast.ASTTypeExpression
	rbCollectTypes(pf.Module, &types)

	if !assert.Len(t, types, 2) {
		return
	}

	for _, texpr := range types {
		assert.Equal(t, texpr.Name, texpr.GetStringSource())
		assert.Equal(t, "Base", texpr.Name)
	}

	assert.Nil(t, types[0].Namespace)
	if assert.NotNil(t, types[1].Namespace) {
		assert.Equal(t, "Reporting", types[1].Namespace.Name)
	}
}

// A type used only in a declaration (no calls in the snippet) must still
// pull its definition into the graph; the exactly-one-node contract on the
// class name is what this guards
func TestRubySearchSnippetPullsSuperclassDefinition(t *testing.T) {
	wspace := cog.NewWorkspace(rubyWorkspace(t, "baseproj"))

	// ruby-lsp indexes the workspace in the background after initialize and
	// answers definitions empty until it lands, so retry until the edge appears
	var root cog.COGNode
	vertices := 0

	for attempt := 0; attempt < 10; attempt++ {
		og, r, err := wspace.SearchSnippet(t.Context(), "types.rb", "class Card < Base")

		if !assert.NoError(t, err) {
			return
		}

		root = r

		adj, err := og.Graph.AdjacencyMap()
		if !assert.NoError(t, err) {
			return
		}

		vertices = len(adj)

		if vertices >= 2 {
			break
		}

		time.Sleep(2 * time.Second)
	}

	class, ok := root.(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected root to be the declaring class, got %T", root) {
		assert.Equal(t, "Card", class.Name.Name)
	}

	assert.GreaterOrEqual(t, vertices, 2, "expected the superclass definition as a vertex")
}
