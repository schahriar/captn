package tests_test

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/schahriar/captn/pkg/common"
	"github.com/stretchr/testify/assert"
)

func TestParallelCollectReturnsResults(t *testing.T) {
	got, err := common.ParallelCollect(context.Background(), []int{1, 2, 3}, func(ctx context.Context, v int) (int, error) {
		return v * 2, nil
	})

	assert.NoError(t, err)
	sort.Ints(got)
	assert.Equal(t, []int{2, 4, 6}, got)
}

func TestParallelCollectCancelsWorkersOnError(t *testing.T) {
	sentinel := errors.New("stop")
	cancelled := make(chan struct{})

	got, err := common.ParallelCollect(context.Background(), []int{1, 2}, func(ctx context.Context, v int) (int, error) {
		if v == 1 {
			return 0, sentinel
		}

		<-ctx.Done()
		close(cancelled)
		return 0, ctx.Err()
	})

	assert.Nil(t, got)
	assert.ErrorIs(t, err, sentinel)

	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("worker did not observe cancellation")
	}
}

func TestParallelCollectReturnsBeforeCancelledWorkerFinishes(t *testing.T) {
	sentinel := errors.New("stop")
	blocked := make(chan struct{})
	fail := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)

	go func() {
		_, err := common.ParallelCollect(context.Background(), []int{1, 2}, func(ctx context.Context, v int) (int, error) {
			if v == 1 {
				<-fail
				return 0, sentinel
			}

			close(blocked)
			<-ctx.Done()
			<-release
			return 0, ctx.Err()
		})
		done <- err
	}()

	select {
	case <-blocked:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}

	close(fail)

	select {
	case err := <-done:
		assert.ErrorIs(t, err, sentinel)
	case <-time.After(time.Second):
		t.Fatal("ParallelCollect waited for cancelled worker to finish")
	}

	close(release)
}

func TestParallelCollectReturnsParentContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	done := make(chan error, 1)

	go func() {
		_, err := common.ParallelCollect(ctx, []int{1}, func(ctx context.Context, v int) (int, error) {
			close(started)
			<-ctx.Done()
			return 0, ctx.Err()
		})
		done <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}

	cancel()

	select {
	case err := <-done:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("ParallelCollect did not return after parent context cancellation")
	}
}
