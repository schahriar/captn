package tests_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/dominikbraun/graph"
	"github.com/schahriar/captn/pkg/cgraph"
	"github.com/stretchr/testify/assert"
)

func stringHash(s string) string { return s }

func newDirectedGraph(t *testing.T, vertices []string, edges [][2]string) cgraph.Graph[string, string] {
	t.Helper()
	g := cgraph.NewGraph[string, string](stringHash, graph.Directed())
	for _, v := range vertices {
		assert.NoError(t, g.AddVertex(v))
	}
	for _, e := range edges {
		assert.NoError(t, g.AddEdge(e[0], e[1]))
	}
	return g
}

func TestDetailedDFS_SingleVertex(t *testing.T) {
	g := newDirectedGraph(t, []string{"a"}, nil)

	var visits []cgraph.DFSVisit[string, string]
	err := g.DetailedDFS("a", func(v cgraph.DFSVisit[string, string]) (bool, error) {
		visits = append(visits, v)
		return false, nil
	})

	assert.NoError(t, err)
	assert.Len(t, visits, 1)

	v := visits[0]
	assert.Equal(t, "a", v.ID)
	assert.Equal(t, "a", v.Vertex)
	assert.Equal(t, 0, v.Depth)
	assert.False(t, v.HasParent)
	assert.False(t, v.HasEdge)
}

func TestDetailedDFS_TreeVisitsAllWithCorrectMetadata(t *testing.T) {
	g := newDirectedGraph(t,
		[]string{"a", "b", "c", "d"},
		[][2]string{{"a", "b"}, {"a", "c"}, {"b", "d"}},
	)

	visited := map[string]cgraph.DFSVisit[string, string]{}
	err := g.DetailedDFS("a", func(v cgraph.DFSVisit[string, string]) (bool, error) {
		_, dup := visited[v.ID]
		assert.False(t, dup, "vertex %q visited more than once", v.ID)
		visited[v.ID] = v
		return false, nil
	})

	assert.NoError(t, err)
	assert.Len(t, visited, 4)

	assert.Equal(t, 0, visited["a"].Depth)
	assert.False(t, visited["a"].HasParent)
	assert.False(t, visited["a"].HasEdge)

	assert.Equal(t, 1, visited["b"].Depth)
	assert.True(t, visited["b"].HasParent)
	assert.Equal(t, "a", visited["b"].Parent)
	assert.True(t, visited["b"].HasEdge)
	assert.Equal(t, "a", visited["b"].Via.Source)
	assert.Equal(t, "b", visited["b"].Via.Target)

	assert.Equal(t, 1, visited["c"].Depth)
	assert.Equal(t, "a", visited["c"].Parent)
	assert.Equal(t, "a", visited["c"].Via.Source)
	assert.Equal(t, "c", visited["c"].Via.Target)

	assert.Equal(t, 2, visited["d"].Depth)
	assert.Equal(t, "b", visited["d"].Parent)
	assert.Equal(t, "b", visited["d"].Via.Source)
	assert.Equal(t, "d", visited["d"].Via.Target)
}

func TestDetailedDFS_StopEarlyHaltsTraversal(t *testing.T) {
	g := newDirectedGraph(t,
		[]string{"a", "b", "c", "d"},
		[][2]string{{"a", "b"}, {"b", "c"}, {"c", "d"}},
	)

	var seen []string
	err := g.DetailedDFS("a", func(v cgraph.DFSVisit[string, string]) (bool, error) {
		seen = append(seen, v.ID)
		return v.ID == "b", nil
	})

	assert.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, seen)
}

func TestDetailedDFS_StopOnFirstVisit(t *testing.T) {
	g := newDirectedGraph(t,
		[]string{"a", "b", "c"},
		[][2]string{{"a", "b"}, {"a", "c"}},
	)

	calls := 0
	err := g.DetailedDFS("a", func(v cgraph.DFSVisit[string, string]) (bool, error) {
		calls++
		return true, nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 1, calls)
}

func TestDetailedDFS_VisitorErrorPropagatesAndStops(t *testing.T) {
	g := newDirectedGraph(t,
		[]string{"a", "b", "c", "d"},
		[][2]string{{"a", "b"}, {"b", "c"}, {"c", "d"}},
	)

	sentinel := errors.New("boom")
	var seen []string
	err := g.DetailedDFS("a", func(v cgraph.DFSVisit[string, string]) (bool, error) {
		seen = append(seen, v.ID)
		if v.ID == "b" {
			return false, sentinel
		}
		return false, nil
	})

	assert.ErrorIs(t, err, sentinel)
	assert.Equal(t, []string{"a", "b"}, seen)
}

func TestDetailedDFS_VisitorErrorOnFirstVisit(t *testing.T) {
	g := newDirectedGraph(t,
		[]string{"a", "b"},
		[][2]string{{"a", "b"}},
	)

	sentinel := errors.New("immediate")
	calls := 0
	err := g.DetailedDFS("a", func(v cgraph.DFSVisit[string, string]) (bool, error) {
		calls++
		return false, sentinel
	})

	assert.ErrorIs(t, err, sentinel)
	assert.Equal(t, 1, calls)
}

func TestDetailedDFS_ErrorTakesPrecedenceWhenStopAlsoTrue(t *testing.T) {
	g := newDirectedGraph(t, []string{"a"}, nil)

	sentinel := errors.New("both")
	err := g.DetailedDFS("a", func(v cgraph.DFSVisit[string, string]) (bool, error) {
		return true, sentinel
	})

	assert.ErrorIs(t, err, sentinel)
}

func TestDetailedDFS_CycleEachVisitedOnce(t *testing.T) {
	g := newDirectedGraph(t,
		[]string{"a", "b", "c"},
		[][2]string{{"a", "b"}, {"b", "c"}, {"c", "a"}},
	)

	counts := map[string]int{}
	err := g.DetailedDFS("a", func(v cgraph.DFSVisit[string, string]) (bool, error) {
		counts[v.ID]++
		return false, nil
	})

	assert.NoError(t, err)
	assert.Equal(t, map[string]int{"a": 1, "b": 1, "c": 1}, counts)
}

func TestDetailedDFS_UnreachableVerticesSkipped(t *testing.T) {
	g := newDirectedGraph(t,
		[]string{"a", "b", "c"},
		[][2]string{{"a", "b"}},
	)

	visited := map[string]bool{}
	err := g.DetailedDFS("a", func(v cgraph.DFSVisit[string, string]) (bool, error) {
		visited[v.ID] = true
		return false, nil
	})

	assert.NoError(t, err)
	assert.True(t, visited["a"])
	assert.True(t, visited["b"])
	assert.False(t, visited["c"])
}

func TestDetailedDFS_DiamondVertexVisitedOnce(t *testing.T) {
	g := newDirectedGraph(t,
		[]string{"a", "b", "c", "d"},
		[][2]string{{"a", "b"}, {"a", "c"}, {"b", "d"}, {"c", "d"}},
	)

	counts := map[string]int{}
	err := g.DetailedDFS("a", func(v cgraph.DFSVisit[string, string]) (bool, error) {
		counts[v.ID]++
		return false, nil
	})

	assert.NoError(t, err)
	assert.Equal(t, 1, counts["d"], "diamond join vertex must only be visited once")
	assert.Len(t, counts, 4)
}

func TestDetailedDFS_MissingStartReturnsError(t *testing.T) {
	g := newDirectedGraph(t, []string{"a"}, nil)

	called := false
	err := g.DetailedDFS("missing", func(v cgraph.DFSVisit[string, string]) (bool, error) {
		called = true
		return false, nil
	})

	assert.Error(t, err)
	assert.False(t, called, "visit should not be invoked when start vertex is missing")
}

func TestDetailedDFS_UndirectedDoesNotRevisitParent(t *testing.T) {
	g := cgraph.NewGraph[string, string](stringHash)
	for _, v := range []string{"a", "b", "c"} {
		assert.NoError(t, g.AddVertex(v))
	}
	assert.NoError(t, g.AddEdge("a", "b"))
	assert.NoError(t, g.AddEdge("b", "c"))

	counts := map[string]int{}
	depths := map[string]int{}
	err := g.DetailedDFS("a", func(v cgraph.DFSVisit[string, string]) (bool, error) {
		counts[v.ID]++
		depths[v.ID] = v.Depth
		return false, nil
	})

	assert.NoError(t, err)
	assert.Equal(t, map[string]int{"a": 1, "b": 1, "c": 1}, counts)
	assert.Equal(t, 0, depths["a"])
	assert.Equal(t, 1, depths["b"])
	assert.Equal(t, 2, depths["c"])
}

func TestDOTWritesUndirectedGraph(t *testing.T) {
	g := cgraph.NewGraph(stringHash)
	assert.NoError(t, g.AddVertex("a", graph.VertexAttribute("label", "Alpha")))
	assert.NoError(t, g.AddVertex("b"))
	assert.NoError(t, g.AddEdge("a", "b", graph.EdgeWeight(7), graph.EdgeAttribute("kind", "calls")))

	var out bytes.Buffer
	assert.NoError(t, cgraph.DOT(&g, &out))

	dot := out.String()
	assert.Contains(t, dot, "strict graph {")
	assert.Contains(t, dot, `"va" [ "label"="Alpha", weight=0 ];`)
	assert.Contains(t, dot, `"vb" [ weight=0 ];`)
	assert.Contains(t, dot, `"va" -- "vb" [ weight=7, "kind"="calls" ];`)
}

func TestDOTWritesDirectedGraph(t *testing.T) {
	g := cgraph.NewGraph(stringHash, graph.Directed())
	assert.NoError(t, g.AddVertex("a"))
	assert.NoError(t, g.AddVertex("b"))
	assert.NoError(t, g.AddEdge("a", "b"))

	var out bytes.Buffer
	assert.NoError(t, cgraph.DOT(&g, &out))

	dot := out.String()
	assert.Contains(t, dot, "strict digraph {")
	assert.Contains(t, dot, `"va" -> "vb" [ weight=0 ];`)
	assert.NotContains(t, dot, `"va" -- "vb"`)
}
