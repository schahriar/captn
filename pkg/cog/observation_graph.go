package cog

import (
	"context"
	"fmt"
	"strings"

	"github.com/schahriar/captn/pkg/cgraph"
)

type ObservationGraph struct {
	Graph *cgraph.Graph[string, COGNode]
}

func (og *ObservationGraph) ExplainWithDepth(ctx context.Context, cog *COG, prov ObservationProvider, n COGNode, depth int) (string, error) {
	if err := prov.ResolveObservationsToGraph(ctx, cog, og, n); err != nil {
		return "", err
	}

	var expln strings.Builder

	err := og.Graph.DetailedDFS(n.GetHash(), func(cur cgraph.DFSVisit[string, COGNode]) (bool, error) {
		// First append the node description
		_, err := expln.WriteString(
			fmt.Sprintf(
				"%v does this %v with the following code \n```%v\n%v\n```",
				cur.Vertex.GetFilePath(),
				safeReadAttr(cur.VertexAttributes(),
					"observation-behavior",
				),
				cur.Vertex.GetLanguage(),
				cur.Vertex.GetSource(),
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
