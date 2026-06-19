package cgraph

import (
	"errors"
	"sort"
	"sync"

	"github.com/dominikbraun/graph"
)

type Graph[K comparable, T any] struct {
	graph.Graph[K, T]
	hash  graph.Hash[K, T]
	attrs map[K]map[string]string
	mu    sync.RWMutex
}

func New[K comparable, T any](hash graph.Hash[K, T], options ...func(*graph.Traits)) Graph[K, T] {
	return Graph[K, T]{
		Graph: graph.New(hash, options...),
		hash:  hash,
		attrs: make(map[K]map[string]string),
	}
}

type DFSVisit[K comparable, T any] struct {
	ID        K
	Vertex    T
	Parent    K
	HasParent bool
	Depth     int
	Via       graph.Edge[K]
	HasEdge   bool

	g *Graph[K, T]
}

func (v DFSVisit[K, T]) VertexAttributes() map[string]string {
	if v.g == nil {
		return map[string]string{}
	}
	return v.g.vertexAttrs(v.ID)
}

func (v DFSVisit[K, T]) EdgeAttributes() map[string]string {
	out := map[string]string{}
	if !v.HasEdge {
		return out
	}
	for key, val := range v.Via.Properties.Attributes {
		out[key] = val
	}
	return out
}

type dfsFrame[K comparable] struct {
	id        K
	parent    K
	hasParent bool
	depth     int
	via       graph.Edge[K]
	hasEdge   bool
}

func (g *Graph[K, T]) DetailedDFS(
	start K,
	// Return true to stop traversal, or a non-nil error to stop and propagate.
	visit func(DFSVisit[K, T]) (bool, error),
) error {
	adj, err := g.AdjacencyMap()
	if err != nil {
		return err
	}

	visited := map[K]bool{
		start: true,
	}

	stack := []dfsFrame[K]{
		{id: start, depth: 0},
	}

	for len(stack) > 0 {
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		vertex, err := g.Vertex(f.id)
		if err != nil {
			return err
		}

		stop, err := visit(DFSVisit[K, T]{
			ID:        f.id,
			Vertex:    vertex,
			Parent:    f.parent,
			HasParent: f.hasParent,
			Depth:     f.depth,
			Via:       f.via,
			HasEdge:   f.hasEdge,
			g:         g,
		})
		if err != nil {
			return err
		}
		if stop {
			return nil
		}

		children := make([]K, 0, len(adj[f.id]))
		for child := range adj[f.id] {
			if visited[child] {
				continue
			}

			visited[child] = true
			children = append(children, child)
		}

		// Apply sort for consistent traversal order
		sort.Slice(children, func(a, b int) bool {
			return a < b
		})

		// Reverse push so pop order matches recursive DFS order.
		for i := len(children) - 1; i >= 0; i-- {
			child := children[i]
			stack = append(stack, dfsFrame[K]{
				id:        child,
				parent:    f.id,
				hasParent: true,
				depth:     f.depth + 1,
				via:       adj[f.id][child],
				hasEdge:   true,
			})
		}
	}

	return nil
}

func (g *Graph[K, T]) AddVertex(value T, options ...func(*graph.VertexProperties)) error {
	k := g.hash(value)

	props := graph.VertexProperties{Attributes: make(map[string]string)}
	for _, opt := range options {
		opt(&props)
	}

	g.mu.Lock()
	if g.attrs[k] == nil {
		g.attrs[k] = make(map[string]string)
	}
	for key, val := range props.Attributes {
		g.attrs[k][key] = val
	}
	g.mu.Unlock()

	err := g.Graph.AddVertex(value, options...)
	if errors.Is(err, graph.ErrVertexAlreadyExists) {
		return nil
	}
	return err
}

func (g *Graph[K, T]) SetEdgeAttribute(source, target K, key, value string) error {
	return g.Graph.UpdateEdge(source, target, graph.EdgeAttribute(key, value))
}

func (g *Graph[K, T]) SetVertexAttribute(h K, key, value string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.attrs[h] == nil {
		g.attrs[h] = make(map[string]string)
	}
	g.attrs[h][key] = value
}

func (g *Graph[K, T]) VertexWithProperties(h K) (T, graph.VertexProperties, error) {
	vertex, props, err := g.Graph.VertexWithProperties(h)
	if err != nil {
		return vertex, props, err
	}

	if props.Attributes == nil {
		props.Attributes = make(map[string]string)
	}

	g.mu.RLock()
	for key, val := range g.attrs[h] {
		props.Attributes[key] = val
	}
	g.mu.RUnlock()

	return vertex, props, nil
}

func (g *Graph[K, T]) vertexAttrs(h K) map[string]string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make(map[string]string, len(g.attrs[h]))
	for key, val := range g.attrs[h] {
		out[key] = val
	}
	return out
}
