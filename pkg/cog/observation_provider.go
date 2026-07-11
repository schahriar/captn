package cog

import (
	"context"
	"errors"

	"github.com/schahriar/captn/pkg/knownerr"
	"github.com/schahriar/captn/pkg/queries"
)

const maxProviderQueryAttempts = 3

type ObservationProvider interface {
	Query(ctx context.Context, cog *Workspace, g *RootedObservationGraph, q queries.PromptQuery) error
}

type ProviderQueryFunc func(ctx context.Context, wspace *Workspace, g *RootedObservationGraph, q queries.PromptQuery) error

// QueryProviderWrapper returns the provider's Query with retries on errors
// marked knownerr.Recoverable. Provider output is nondeterministic (e.g. an
// LLM emitting malformed JSON); successful observations are cached by the
// provider, so a retry only re-queries what failed.
func QueryProviderWrapper[P ObservationProvider](prov P) ProviderQueryFunc {
	return func(ctx context.Context, wspace *Workspace, g *RootedObservationGraph, q queries.PromptQuery) error {
		var err error

		for attempt := 0; attempt < maxProviderQueryAttempts; attempt++ {
			if cerr := ctx.Err(); cerr != nil {
				return cerr
			}

			err = prov.Query(ctx, wspace, g, q)

			var rec knownerr.Recoverable
			if err == nil || !errors.As(err, &rec) || !rec.IsRecoverable() {
				return knownerr.LogError(err)
			}
		}

		return knownerr.LogError(err)
	}
}
