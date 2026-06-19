package cgraph

import (
	"fmt"
	"io"
	"strings"
)

func DOT[K ~string, T any](g *Graph[K, T], w io.Writer) error {
	graphType := "strict graph"
	edgeOp := "--"
	if g.Traits().IsDirected {
		graphType = "strict digraph"
		edgeOp = "->"
	}

	adjacency, err := g.AdjacencyMap()
	if err != nil {
		return fmt.Errorf("adjacency map: %w", err)
	}

	fmt.Fprintf(w, "%s {\n\n", graphType)

	for vertex, edges := range adjacency {
		attrs := g.vertexAttrs(vertex)
		fmt.Fprintf(w, "\t%q [ %sweight=0 ];\n\n", string(vertex), formatAttrs(attrs))

		for target, edge := range edges {
			fmt.Fprintf(w, "\t%q %s %q [ weight=%v%s ];\n\n", string(vertex), edgeOp, string(target), edge.Properties.Weight, formatEdgeAttrs(edge.Properties.Attributes))
		}
	}

	fmt.Fprintf(w, "}\n")
	return nil
}

func formatAttrs(attrs map[string]string) string {
	if len(attrs) == 0 {
		return ""
	}
	var sb strings.Builder
	for k, v := range attrs {
		fmt.Fprintf(&sb, "%q=%q, ", k, v)
	}
	return sb.String()
}

func formatEdgeAttrs(attrs map[string]string) string {
	if len(attrs) == 0 {
		return ""
	}
	var sb strings.Builder
	for k, v := range attrs {
		fmt.Fprintf(&sb, ", %q=%q", k, v)
	}
	return sb.String()
}
