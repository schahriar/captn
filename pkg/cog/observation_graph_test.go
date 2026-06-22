package cog_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/dominikbraun/graph"
	"github.com/schahriar/captn/pkg/cgraph"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/common"
	"github.com/stretchr/testify/assert"
)

type dotTestNode struct {
	hash     uint32
	path     string
	language string
	label    string
}

func newDotTestNode(hash uint32, path, language, label string) dotTestNode {
	return dotTestNode{
		hash:     hash,
		path:     path,
		language: language,
		label:    label,
	}
}

func (n dotTestNode) GetHash() uint32 {
	return n.hash
}

func (n dotTestNode) GetFilePath() string {
	return n.path
}

func (n dotTestNode) GetLanguage() string {
	return n.language
}

func (n dotTestNode) GetStringSource() string {
	return ""
}

func (n dotTestNode) ListDependencies(ctx context.Context) (common.ResolvedDependencies, error) {
	return nil, nil
}

func (n dotTestNode) String() string {
	return n.label
}

func TestObservationGraphWriteDOTAddsVizNodeAttributes(t *testing.T) {
	g := cgraph.NewGraph[uint32, cog.COGNode](cog.NodeHasher)
	og := cog.NewObservationGraph(&g)

	node := newDotTestNode(42, "/workspace/vendor/example/mod/file.go", "golang", "CallExpression Symbol:x(1)")

	assert.NoError(t, og.Graph.AddVertex(node, graph.VertexAttribute("import_type", "package")))

	var out bytes.Buffer
	assert.NoError(t, og.WriteDOT(&out))

	dot := out.String()
	assert.Contains(t, dot, `"label"="CallExpression Symbol:x(1)"`)
	assert.Contains(t, dot, `"import_type"="dependency"`)
	assert.Contains(t, dot, `"file_type"="golang"`)
	assert.Contains(t, dot, `"node_type"="cog_test.dotTestNode"`)
}
