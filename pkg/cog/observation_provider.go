package cog

import (
	"context"

	"github.com/schahriar/captn/pkg/queries"
)

type ObservationProvider interface {
	Query(ctx context.Context, cog *Workspace, g *RootedObservationGraph, q queries.PromptQuery) error
}
