package cog

import (
	"context"

	"github.com/schahriar/captn/pkg/common"
)

type ObservationProvider interface {
	GetObservationFromSource(ctx context.Context, r *common.FileRange) (common.ObservationSchema, error)
	ResolveObservationsToGraph(ctx context.Context, cog *COG, g *ObservationGraph, root COGNode) error
}
