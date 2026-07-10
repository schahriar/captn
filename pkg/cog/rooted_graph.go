package cog

type RootedObservationGraph struct {
	*ObservationGraph
	Root COGNode
}

func NewRootedObservationGraph(g *ObservationGraph, root COGNode) *RootedObservationGraph {
	return &RootedObservationGraph{ObservationGraph: g, Root: root}
}
