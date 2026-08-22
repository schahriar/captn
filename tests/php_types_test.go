package tests_test

import (
	"os"
	"testing"
	"time"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/stretchr/testify/assert"
)

// Every TypeExpression must span its identifier alone so a definition
// resolves onto exactly one node, and composites (nullable, union) must fold
// flat with the first named type as the head
func TestPHPTypeShapes(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/php/baseproj/types.php")

	var types []*ast.ASTTypeExpression
	phpCollectTypes(pf.Module, &types)

	for _, texpr := range types {
		assert.Equal(t, texpr.Name, texpr.GetStringSource(), "type range must span the identifier alone")
	}

	fn, ok := pf.Module.Block.Children()[3].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected the pick function") {
		return
	}

	if !assert.Len(t, fn.Arguments, 2) {
		return
	}

	// ?Card folds to its inner named type
	if assert.NotNil(t, fn.Arguments[0].Type) {
		assert.Equal(t, "Card", fn.Arguments[0].Type.Name)
		assert.Empty(t, fn.Arguments[0].Type.Arguments)
	}

	// Base|Renderer folds flat: head Base, Renderer as its argument
	if assert.NotNil(t, fn.Arguments[1].Type) {
		assert.Equal(t, "Base", fn.Arguments[1].Type.Name)
		if assert.Len(t, fn.Arguments[1].Type.Arguments, 1) {
			assert.Equal(t, "Renderer", fn.Arguments[1].Type.Arguments[0].Name)
		}
	}

	// Card|null keeps the named type as head with the primitive folded in
	if assert.NotNil(t, fn.ReturnType) {
		assert.Equal(t, "Card", fn.ReturnType.Name)
	}
}

// A type used only in a signature (the snippet calls nothing) must still
// pull its definition into the graph; the exactly-one-node contract on the
// declared names is what this guards
func TestPHPSearchSnippetPullsSignatureTypes(t *testing.T) {
	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	wspace := cog.NewWorkspace(cwd)

	// intelephense indexes in the background; retry until the type
	// definitions resolve into vertices
	var root cog.COGNode
	vertices := 0

	for attempt := 0; attempt < 10; attempt++ {
		og, r, err := wspace.SearchSnippet(t.Context(), "./fixtures/php/baseproj/types.php",
			"function pick(?Card $card, Base|Renderer $fallback): Card|null")

		if !assert.NoError(t, err) {
			return
		}

		root = r

		adj, err := og.Graph.AdjacencyMap()
		if !assert.NoError(t, err) {
			return
		}

		vertices = len(adj)

		if vertices >= 4 {
			break
		}

		time.Sleep(2 * time.Second)
	}

	fn, ok := root.(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected root to be the declaring function, got %T", root) {
		assert.Equal(t, "pick", fn.Name.Name)
	}

	// Card, Base and Renderer definitions each become a vertex
	assert.GreaterOrEqual(t, vertices, 4, "expected the signature types as vertices")
}
