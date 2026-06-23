package tests_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/schahriar/captn/pkg/cog"
	"github.com/stretchr/testify/assert"
)

func TestCOGLoadFilesDeduplicatesConcurrentLoads(t *testing.T) {
	cwd, err := os.Getwd()
	assert.NoError(t, err)

	c := cog.NewCOG(cwd)
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
	c := cog.NewCOG(workspace)

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
