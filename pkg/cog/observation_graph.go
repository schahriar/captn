package cog

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/cgraph"
	"github.com/schahriar/captn/pkg/common"
)

type ObservationGraph struct {
	Graph *cgraph.Graph[uint32, COGNode]
}

func NewObservationGraph(g *cgraph.Graph[uint32, COGNode]) *ObservationGraph {
	return &ObservationGraph{Graph: g}
}

func (og *ObservationGraph) WriteDOT(w io.Writer) error {
	graphType := "strict graph"
	edgeOp := "--"
	if og.Graph.Traits().IsDirected {
		graphType = "strict digraph"
		edgeOp = "->"
	}

	adjacency, err := og.Graph.AdjacencyMap()
	if err != nil {
		return fmt.Errorf("adjacency map: %w", err)
	}

	fmt.Fprintf(w, "%s {\n\n", graphType)

	vertices := make([]uint32, 0, len(adjacency))
	for vertex := range adjacency {
		vertices = append(vertices, vertex)
	}
	sort.Slice(vertices, func(i, j int) bool {
		return vertices[i] < vertices[j]
	})

	for _, vertex := range vertices {
		node, props, err := og.Graph.VertexWithProperties(vertex)
		if err != nil {
			return err
		}

		attrs := observationGraphNodeAttrs(node, props.Attributes)
		fmt.Fprintf(w, "\t\"v%v\" [ %sweight=0 ];\n\n", vertex, formatDOTAttrs(attrs))

		targets := make([]uint32, 0, len(adjacency[vertex]))
		for target := range adjacency[vertex] {
			targets = append(targets, target)
		}
		sort.Slice(targets, func(i, j int) bool {
			return targets[i] < targets[j]
		})

		for _, target := range targets {
			edge := adjacency[vertex][target]
			fmt.Fprintf(w, "\t\"v%v\" %s \"v%v\" [ weight=%v%s ];\n\n", vertex, edgeOp, target, edge.Properties.Weight, formatDOTEdgeAttrs(edge.Properties.Attributes))
		}
	}

	fmt.Fprintf(w, "}\n")
	return nil
}

func observationGraphNodeAttrs(node COGNode, attrs map[string]string) map[string]string {
	out := make(map[string]string, len(attrs)+4)
	for key, val := range attrs {
		out[key] = val
	}

	if _, ok := out["label"]; !ok {
		out["label"] = observationGraphNodeLabel(node)
	}

	out["import_type"] = observationGraphImportType(node, out["import_type"])
	out["file_type"] = node.GetLanguage()

	if _, ok := out["node_type"]; !ok {
		out["node_type"] = observationGraphNodeType(node)
	}

	return out
}

func observationGraphNodeLabel(node COGNode) string {
	if stringer, ok := node.(fmt.Stringer); ok {
		return stringer.String()
	}

	if path := node.GetFilePath(); path != "" {
		return filepath.Base(path)
	}

	return fmt.Sprintf("%T", node)
}

func observationGraphNodeType(node COGNode) string {
	if astNode, ok := node.(ast.ASTNode); ok {
		return astNode.Kind()
	}

	return fmt.Sprintf("%T", node)
}

func observationGraphImportType(node COGNode, current string) string {
	switch common.DependencyType(current) {
	case common.LocalDependency:
		return string(common.LocalDependency)
	case common.StandardLibraryDependency:
		return string(common.StandardLibraryDependency)
	case common.PackageDependency:
		return "dependency"
	}

	path := filepath.ToSlash(filepath.Clean(node.GetFilePath()))
	switch {
	case strings.Contains(path, "/pkg/mod/golang.org/toolchain@") && strings.Contains(path, "/src/"):
		return string(common.StandardLibraryDependency)
	case strings.Contains(path, "/pkg/mod/"), strings.Contains(path, "/vendor/"):
		return "dependency"
	default:
		return string(common.LocalDependency)
	}
}

func formatDOTAttrs(attrs map[string]string) string {
	if len(attrs) == 0 {
		return ""
	}

	keys := make([]string, 0, len(attrs))
	for key := range attrs {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&sb, "%q=%q, ", key, attrs[key])
	}
	return sb.String()
}

func formatDOTEdgeAttrs(attrs map[string]string) string {
	if len(attrs) == 0 {
		return ""
	}

	keys := make([]string, 0, len(attrs))
	for key := range attrs {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&sb, ", %q=%q", key, attrs[key])
	}
	return sb.String()
}

func (og *ObservationGraph) WriteToFile(ctx context.Context, path string) error {
	f, err := os.Create(path)

	if err != nil {
		return err
	}

	defer f.Close()

	if err := og.WriteDOT(f); err != nil {
		return err
	}

	return nil
}

func (og *ObservationGraph) ExplainWithDepth(ctx context.Context, cog *COG, prov ObservationProvider, n COGNode, depth int) (string, error) {
	if cog == nil {
		return "", fmt.Errorf("expected instance of COG received nil")
	}

	if n == nil {
		return "", fmt.Errorf("expected instance of COGNode as root node received nil")
	}

	if err := prov.ResolveObservationsToGraph(ctx, cog, og, n); err != nil {
		return "", err
	}

	var expln strings.Builder

	og.WriteToFile(ctx, "./graph.gv")

	err := og.Graph.DetailedDFS(n.GetHash(), func(cur cgraph.DFSVisit[uint32, COGNode]) (bool, error) {
		// First append the node description
		_, err := expln.WriteString(
			fmt.Sprintf(
				"%v does this %v with the following code \n```%v\n%v\n```",
				cur.Vertex.GetFilePath(),
				safeReadAttr(cur.VertexAttributes(),
					"observation-behavior",
				),
				cur.Vertex.GetLanguage(),
				cur.Vertex.GetStringSource(),
			),
		)

		if err != nil {
			return true, err
		}

		// Append edge connection
		if cur.HasParent {
			expln.WriteString(safeReadAttr(cur.Via.Properties.Attributes, "observation-behavior"))

			par, err := og.Graph.Vertex(cur.Parent)

			if err != nil {
				return true, err
			}

			_, err = expln.WriteString(
				fmt.Sprintf(
					"%v uses %v with the following behavior\n '''%v'''\n",
					par.GetFilePath(),
					cur.Vertex.GetFilePath(),
					safeReadAttr(cur.EdgeAttributes(),
						"observation-behavior",
					),
				),
			)

			if err != nil {
				return true, err
			}
		}

		return false, nil
	})

	if err != nil {
		return "", err
	}

	return expln.String(), nil
}
