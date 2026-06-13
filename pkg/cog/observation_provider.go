package cog

import (
	"context"

	"github.com/dominikbraun/graph"
	"github.com/schahriar/captn/pkg/common"
)

type ObservationProvider interface {
	GetObservationFromSource(ctx context.Context, r *common.FileRange) (common.ObservationSchema, error)
	ResolveObservationsToGraph(ctx context.Context, g graph.Graph[string, COGNode], root COGNode) error
}
