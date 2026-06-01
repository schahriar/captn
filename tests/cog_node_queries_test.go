package tests_test

import (
	"fmt"
	"testing"

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

	for _, node := range nodes {
		fmt.Println(node.String(), node.DebugPosition())
	}
}
