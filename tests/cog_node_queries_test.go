package tests_test

import (
	"testing"

	"github.com/gkampitakis/go-snaps/snaps"
	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/common"
	"github.com/stretchr/testify/assert"
)

func TestCOGRangeQueryModule(t *testing.T) {
	pf := parseSimple(t)

	rng, err := common.NewFileRangeAutoBytePosition(pf.Source, 2, 5, 2, 7)

	assert.NoError(t, err)

	nodes := pf.FindNodesWithinRange(common.NewFileRange(
		pf.Source,
		rng.Start,
		rng.End,
	))

	assert.Len(t, nodes, 1)

	node := nodes[0]

	sym, ok := node.(*ast.ASTSymbol)
	assert.True(t, ok)

	assert.Equal(t, "x", sym.Name)

	rng, err = common.NewFileRangeAutoBytePosition(pf.Source, 2, 0, 4, 1)

	assert.NoError(t, err)

	nodes = pf.FindNodesWithinRange(common.NewFileRange(
		pf.Source,
		rng.Start,
		rng.End,
	))

	kinds := common.Map(nodes, func(node ast.ASTNode) string {
		return node.Kind()
	})

	assert.Contains(t, kinds, "FuncExpression")
	assert.Contains(t, kinds, "FuncArgument")
	assert.Contains(t, kinds, "Return")
	assert.Contains(t, kinds, "CallExpression")
}

func TestCOGRangeQueryFindsMethodDefinitionSymbol(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/golang/baseproj/method.go")

	rng, err := common.NewFileRangeAutoBytePosition(pf.Source, 4, 17, 4, 25)
	assert.NoError(t, err)

	nodes := pf.FindNodesWithinRange(rng)
	if !assert.Len(t, nodes, 1) {
		return
	}

	sym, ok := nodes[0].(*ast.ASTSymbol)
	if assert.True(t, ok, "expected definition range to resolve to symbol") {
		assert.Equal(t, "Describe", sym.Name)
	}
}

func TestCOGResolveImports(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/golang/multidep/cmd/main.go")

	imps, err := pf.ListDependencies(t.Context())

	assert.NoError(t, err)

	snaps.MatchYAML(t, imps)
}
