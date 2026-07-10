package tests_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/common"
	"github.com/stretchr/testify/assert"
)

func TestCOGLoadFilesDeduplicatesConcurrentLoads(t *testing.T) {
	cwd, err := os.Getwd()
	assert.NoError(t, err)

	c := cog.NewWorkspace(cwd)
	files := make([]string, 32)
	for i := range files {
		files[i] = "./fixtures/golang/baseproj/simple.go"
	}

	loaded, err := c.LoadFiles(t.Context(), files)
	assert.NoError(t, err)
	if !assert.Len(t, loaded, len(files)) {
		return
	}

	first := loaded[0]
	for _, f := range loaded[1:] {
		assert.Same(t, first, f)
	}
}

func TestCOGLoadFileClearsInflightAfterError(t *testing.T) {
	workspace := t.TempDir()
	c := cog.NewWorkspace(workspace)

	_, err := c.LoadFile(t.Context(), "main.go")
	assert.Error(t, err)

	err = os.WriteFile(
		filepath.Join(workspace, "main.go"),
		[]byte("package main\n\nfunc main() {}\n"),
		0644,
	)
	assert.NoError(t, err)

	loaded, err := c.LoadFile(t.Context(), "main.go")
	assert.NoError(t, err)
	assert.NotNil(t, loaded)
}

func TestCOGPersistReturns(t *testing.T) {
	c := cog.NewWorkspace(t.TempDir())
	done := make(chan error, 1)

	go func() {
		done <- c.Persist()
	}()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("COG Persist did not return")
	}
}

func TestOpenCOGLoadsPersistedCache(t *testing.T) {
	workspace := t.TempDir()
	h := common.PrimaryHash("persisted-observation")
	o := common.NewObservationSchema(h.String(), "persisted answer")
	co := cog.NewCOGObservation(o, []*common.FileRange{})

	c := cog.NewWorkspace(workspace)
	c.ObservationCache[h] = co
	assert.NoError(t, c.Persist())

	loaded, err := cog.OpenWorkspace(workspace)
	assert.NoError(t, err)
	assert.Equal(t, co, loaded.ObservationCache[h])
}
