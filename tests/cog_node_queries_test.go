package tests_test

import (
	"fmt"
	"testing"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/common"
	"github.com/stretchr/testify/assert"
)

func TestCOGRangeQueryModule(t *testing.T) {
	pf := parseSimple(t)

	rng, err := common.NewFileRangeAutoBytePosition(pf.Source, 2, 5, 2, 7)

	assert.NoError(t, err)

	nodes := pf.QueryNodesWithinRange(common.NewFileRange(
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

	nodes = pf.QueryNodesWithinRange(common.NewFileRange(
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

func TestCOGResolveImports(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/golang/multidep/cmd/main.go")

	locs, err := pf.ListImports(t.Context())

	assert.NoError(t, err)

	fmt.Println(locs)
}
