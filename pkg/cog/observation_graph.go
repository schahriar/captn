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
	"github.com/schahriar/captn/pkg/languages"
	"github.com/schahriar/captn/pkg/queries"
)

type ObservationGraph struct {
	Graph *cgraph.Graph[common.HashType, COGNode]
}

func NewObservationGraph(g *cgraph.Graph[common.HashType, COGNode]) *ObservationGraph {
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

	vertices := make([]common.HashType, 0, len(adjacency))
	for vertex := range adjacency {
		vertices = append(vertices, vertex)
	}
	sort.Slice(vertices, func(i, j int) bool {
		return vertices[i].Sum() < vertices[j].Sum()
	})

	for _, vertex := range vertices {
		node, props, err := og.Graph.VertexWithProperties(vertex)
		if err != nil {
			return err
		}

		attrs := observationGraphNodeAttrs(node, props.Attributes)
		fmt.Fprintf(w, "\t\"v%v\" [ %sweight=0 ];\n\n", vertex, formatDOTAttrs(attrs))

		targets := make([]common.HashType, 0, len(adjacency[vertex]))
		for target := range adjacency[vertex] {
			targets = append(targets, target)
		}
		sort.Slice(targets, func(i, j int) bool {
			return targets[i].Sum() < targets[j].Sum()
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

	// Vertices added without an import_type attribute are classified by
	// their language from the file path alone
	path := node.GetFilePath()

	if lang, ok := languages.ForExtension(filepath.Ext(path)); ok {
		classified := lang.ClassifyImportType(common.NewSource("", path, nil))

		if classified == common.PackageDependency {
			return "dependency"
		}

		return string(classified)
	}

	return string(common.LocalDependency)
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

func nodeLocation(n COGNode) string {
	if astNode, ok := n.(ast.ASTNode); ok {
		if pos := astNode.GetPosition(); pos != nil {
			return pos.String()
		}
	}

	return n.GetFilePath()
}

func (og *ObservationGraph) QueryWithDepth(ctx context.Context, wspace *Workspace, prov ObservationProvider, n COGNode, q queries.PromptQuery, depth int) (string, error) {
	if wspace == nil {
		return "", fmt.Errorf("expected instance of Workspace received nil")
	}

	if n == nil {
		return "", fmt.Errorf("expected instance of COGNode as root node received nil")
	}

	rog := NewRootedObservationGraph(og, n)

	if err := QueryProviderWrapper(prov)(ctx, wspace, rog, q); err != nil {
		return "", err
	}

	var expln strings.Builder

	og.WriteToFile(ctx, "./graph.gv")

	err := og.Graph.DetailedDFS(n.GetHash(), func(cur cgraph.DFSVisit[common.HashType, COGNode]) (bool, error) {
		if _, err := fmt.Fprintf(&expln, "%v does the following:\n%v\n",
			nodeLocation(cur.Vertex),
			safeReadAttr(cur.VertexAttributes(), "observation-answer"),
		); err != nil {
			return true, err
		}

		if cur.HasParent {
			par, err := og.Graph.Vertex(cur.Parent)

			if err != nil {
				return true, err
			}

			if _, err := fmt.Fprintf(&expln, "%v uses %v with the following answer:\n%v\n",
				nodeLocation(par),
				nodeLocation(cur.Vertex),
				safeReadAttr(cur.EdgeAttributes(), "observation-answer"),
			); err != nil {
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

type GraphWithRoot struct {
	Graph *ObservationGraph
	Root  COGNode
}

func normalizeEdgeKey(a, b common.HashType) [2]common.HashType {
	if a.String() <= b.String() {
		return [2]common.HashType{a, b}
	}
	return [2]common.HashType{b, a}
}

func MultiGraphQueryWithDepth(ctx context.Context, wspace *Workspace, prov ObservationProvider, items []GraphWithRoot, q queries.PromptQuery, depth int) (string, error) {
	if wspace == nil {
		return "", fmt.Errorf("expected instance of Workspace received nil")
	}

	type vertexEntry struct {
		node   COGNode
		answer string
	}

	type edgeEntry struct {
		source COGNode
		target COGNode
		answer string
	}

	vseen := map[common.HashType]vertexEntry{}
	eseen := map[[2]common.HashType]edgeEntry{}

	query := QueryProviderWrapper(prov)

	for _, item := range items {
		if item.Graph == nil || item.Root == nil {
			continue
		}

		rog := NewRootedObservationGraph(item.Graph, item.Root)

		if err := query(ctx, wspace, rog, q); err != nil {
			return "", err
		}

		g := item.Graph.Graph

		adj, err := g.AdjacencyMap()
		if err != nil {
			return "", err
		}

		for h := range adj {
			if _, ok := vseen[h]; ok {
				continue
			}

			node, props, err := g.VertexWithProperties(h)
			if err != nil {
				return "", err
			}

			// banstructlit:ignore
			vseen[h] = vertexEntry{
				node:   node,
				answer: safeReadAttr(props.Attributes, "observation-answer"),
			}
		}

		edgeKeys, err := g.Edges()
		if err != nil {
			return "", err
		}

		for _, ek := range edgeKeys {
			key := normalizeEdgeKey(ek.Source, ek.Target)
			if _, ok := eseen[key]; ok {
				continue
			}

			e, err := g.Edge(ek.Source, ek.Target)
			if err != nil {
				return "", err
			}

			// banstructlit:ignore
			eseen[key] = edgeEntry{
				source: e.Source,
				target: e.Target,
				answer: safeReadAttr(e.Properties.Attributes, "observation-answer"),
			}
		}
	}

	vlist := make([]vertexEntry, 0, len(vseen))
	for _, v := range vseen {
		vlist = append(vlist, v)
	}
	sort.Slice(vlist, func(i, j int) bool {
		return nodeLocation(vlist[i].node) < nodeLocation(vlist[j].node)
	})

	elist := make([]edgeEntry, 0, len(eseen))
	for _, e := range eseen {
		elist = append(elist, e)
	}
	sort.Slice(elist, func(i, j int) bool {
		li, lj := nodeLocation(elist[i].source), nodeLocation(elist[j].source)
		if li != lj {
			return li < lj
		}
		return nodeLocation(elist[i].target) < nodeLocation(elist[j].target)
	})

	var expln strings.Builder

	for _, v := range vlist {
		if _, err := fmt.Fprintf(&expln, "%v does the following:\n%v\n",
			nodeLocation(v.node),
			v.answer,
		); err != nil {
			return "", err
		}
	}

	for _, e := range elist {
		if _, err := fmt.Fprintf(&expln, "%v uses %v with the following answer:\n%v\n",
			nodeLocation(e.source),
			nodeLocation(e.target),
			e.answer,
		); err != nil {
			return "", err
		}
	}

	return expln.String(), nil
}
