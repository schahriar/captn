package tests_test

import (
	"context"
	"errors"
	"testing"

	"github.com/schahriar/captn/pkg/cgraph"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/knownerr"
	"github.com/schahriar/captn/pkg/queries"
	"github.com/stretchr/testify/assert"
)

type fakeObservationProvider struct {
	calls int
	errs  []error
}

func (p *fakeObservationProvider) Query(ctx context.Context, wspace *cog.Workspace, g *cog.RootedObservationGraph, q queries.PromptQuery) error {
	var err error
	if p.calls < len(p.errs) {
		err = p.errs[p.calls]
	}
	p.calls++
	return err
}

func newRetryTestGraph(t *testing.T) *cog.RootedObservationGraph {
	g := cgraph.NewGraph[common.HashType, cog.COGNode](cog.NodeHasher)
	og := cog.NewObservationGraph(&g)
	node := newDotTestNode(common.HashType{1, 0, 0, 0}, "/tmp/file.go", "golang", "n")
	assert.NoError(t, og.Graph.AddVertex(node))
	return cog.NewRootedObservationGraph(og, node)
}

func TestQueryProviderWrapperRetriesRecoverableErrors(t *testing.T) {
	prov := &fakeObservationProvider{errs: []error{
		knownerr.NewProviderOutputError(errors.New("malformed structured output")),
		nil,
	}}

	err := cog.QueryProviderWrapper(prov)(context.Background(), cog.NewWorkspace(t.TempDir()), newRetryTestGraph(t), queries.NewExplainBehaviorQuery())

	assert.NoError(t, err)
	assert.Equal(t, 2, prov.calls)
}

func TestQueryProviderWrapperDoesNotRetryUnrecoverableErrors(t *testing.T) {
	prov := &fakeObservationProvider{errs: []error{
		errors.New("claude binary not found"),
		nil,
	}}

	err := cog.QueryProviderWrapper(prov)(context.Background(), cog.NewWorkspace(t.TempDir()), newRetryTestGraph(t), queries.NewExplainBehaviorQuery())

	assert.ErrorContains(t, err, "claude binary not found")
	assert.Equal(t, 1, prov.calls)
}

func TestQueryProviderWrapperGivesUpAfterMaxAttempts(t *testing.T) {
	provErr := knownerr.NewProviderOutputError(errors.New("still malformed"))
	prov := &fakeObservationProvider{errs: []error{provErr, provErr, provErr, provErr}}

	err := cog.QueryProviderWrapper(prov)(context.Background(), cog.NewWorkspace(t.TempDir()), newRetryTestGraph(t), queries.NewExplainBehaviorQuery())

	assert.ErrorContains(t, err, "still malformed")
	assert.Equal(t, 3, prov.calls)
}

func TestQueryProviderWrapperStopsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	prov := &fakeObservationProvider{}

	err := cog.QueryProviderWrapper(prov)(ctx, cog.NewWorkspace(t.TempDir()), newRetryTestGraph(t), queries.NewExplainBehaviorQuery())

	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 0, prov.calls)
}
