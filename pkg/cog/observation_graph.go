package cog

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/schahriar/captn/pkg/cgraph"
)

type ObservationGraph struct {
	Graph *cgraph.Graph[uint32, COGNode]
}

func NewObservationGraph(g *cgraph.Graph[uint32, COGNode]) *ObservationGraph {
	return &ObservationGraph{Graph: g}
}

func (og *ObservationGraph) WriteToFile(ctx context.Context, path string) error {
	f, err := os.Create(path)

	if err != nil {
		return err
	}

	defer f.Close()

	if err := cgraph.DOT(og.Graph, f); err != nil {
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
