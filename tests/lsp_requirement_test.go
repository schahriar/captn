package tests_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/languages"
	"github.com/schahriar/captn/pkg/lsp"
	"github.com/stretchr/testify/assert"
)

// One pass over the requirement flow with the real Go language support: with
// gopls out of reach the error names the command that installs it, and dropping
// the binary into GOBIN puts the next query back to work without a restart,
// which is the whole point of captn not installing anything itself.
func TestRequireLSPServer(t *testing.T) {
	assert.NoError(t, cog.RequireLSPServer(context.Background(), lsp.ServerRequirement{}))

	req := languages.Golang.GetLSPServerRequirement()

	assert.Equal(t, "gopls", req.Name)
	assert.Equal(t, "go install golang.org/x/tools/gopls@latest", req.InstallCommand)

	// Renamed so this test does not share the process-wide memo of located
	// servers with the queries the other tests run.
	req.Name = t.Name()

	gobin := t.TempDir()
	t.Setenv("GOBIN", gobin)
	t.Setenv("PATH", toolchainOnlyPath(t))

	err := cog.RequireLSPServer(context.Background(), req)

	assert.ErrorContains(t, err, "gopls")
	assert.ErrorContains(t, err, req.InstallCommand)
	// A search fans out over matches and skips the ones it cannot resolve, so it
	// tells this failure apart by its sentinel to answer with it instead.
	assert.ErrorIs(t, err, lsp.ErrServerMissing)

	assert.NoError(t, os.WriteFile(filepath.Join(gobin, "gopls"), nil, 0o755))

	// A cancelled caller must not make an installed server look missing, or a
	// timed-out search would ask for an install that already happened.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assert.NoError(t, cog.RequireLSPServer(ctx, req))
}

// toolchainOnlyPath is a PATH holding the go command and nothing else, so a
// gopls installed on the machine running the tests cannot answer for the one
// this test plants in GOBIN.
func toolchainOnlyPath(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	command, err := exec.LookPath("go")

	assert.NoError(t, err)
	assert.NoError(t, os.Symlink(command, filepath.Join(dir, "go")))

	return dir
}
