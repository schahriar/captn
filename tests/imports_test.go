package tests_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/lsp"
	"github.com/stretchr/testify/assert"
)

func TestNewResolvedDependencyFromURIUsesExactExternalRange(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "dep.go")
	buf := []byte("package main\n\nfunc Dep() {}\n")
	assert.NoError(t, os.WriteFile(path, buf, 0644))

	internalSource := common.NewSource(workspace, filepath.Join(workspace, "main.go"), []byte("package main\n"))
	internal, err := common.NewFileRangeAutoBytePosition(internalSource, 0, 0, 0, 7)
	assert.NoError(t, err)

	dep, err := common.NewResolvedDependencyFromURI(
		context.Background(),
		workspace,
		internal,
		lsp.FileURI(path),
		2,
		5,
		2,
		8,
		func(src *common.Source) common.DependencyType {
			assert.Equal(t, "dep.go", src.Path)
			return common.LocalDependency
		},
	)

	assert.NoError(t, err)
	assert.Equal(t, common.LocalDependency, dep.Type)
	assert.Same(t, internal, dep.Internal)
	assert.Equal(t, "dep.go", dep.External.Source.Path)
	assert.Equal(t, [2]int{19, 22}, dep.External.GetByteRange())
}
