package tests_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/common"
	"github.com/stretchr/testify/assert"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	base := []string{"-c", "user.name=Test", "-c", "user.email=test@example.com"}
	cmd := exec.Command("git", append(base, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	assert.NoError(t, err, "git %v: %s", args, out)
	return string(out)
}

func appendObservation(t *testing.T, dir string, author string, key string, answer string) {
	t.Helper()
	w := cog.NewWorkspace(dir)
	w.ActiveAuthor = author
	w.SetObservation(common.PrimaryHash(key), cog.NewCOGObservation(
		common.NewObservationSchema("s0", answer),
		nil,
	))
	b, err := w.Marshal()
	assert.NoError(t, err)

	f, err := os.OpenFile(filepath.Join(dir, "captn.cog"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	assert.NoError(t, err)
	_, err = f.Write(b)
	assert.NoError(t, err)
	assert.NoError(t, f.Close())
}

// Pins the decree: concurrent captn.cog appends on separate branches must
// merge without conflicts. OpenWorkspace registers a union merge attribute
// in the repository's local git attributes; union merges are safe because
// loading tolerates the duplicate lines they can produce.
func TestWorkspaceConcurrentBranchesMergeCleanly(t *testing.T) {
	dir := t.TempDir()
	gitIn(t, dir, "init")

	appendObservation(t, dir, "Base <base@example.com>", "observation-base", "base answer")
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-m", "base")

	_, err := cog.OpenWorkspace(dir)
	assert.NoError(t, err)

	attributes, err := os.ReadFile(filepath.Join(dir, ".git", "info", "attributes"))
	if !assert.NoError(t, err, "OpenWorkspace must register the merge attribute") {
		return
	}
	assert.Contains(t, string(attributes), "captn.cog merge=union")

	gitIn(t, dir, "checkout", "-b", "alice")
	appendObservation(t, dir, "Alice <alice@example.com>", "observation-alice", "alice answer")
	gitIn(t, dir, "commit", "-am", "alice observation")

	gitIn(t, dir, "checkout", "-")
	gitIn(t, dir, "checkout", "-b", "bob")
	appendObservation(t, dir, "Bob <bob@example.com>", "observation-bob", "bob answer")
	gitIn(t, dir, "commit", "-am", "bob observation")

	gitIn(t, dir, "merge", "alice")

	status := gitIn(t, dir, "status", "--porcelain")
	assert.Empty(t, strings.TrimSpace(status), "merge must leave a clean tree")

	merged, err := os.ReadFile(filepath.Join(dir, "captn.cog"))
	assert.NoError(t, err)

	loaded := cog.NewWorkspace(dir)
	assert.NoError(t, loaded.Unmarshal(merged))
	assert.Len(t, loaded.ObservationCache, 3)
	assert.Equal(t, "alice answer", loaded.ObservationCache[common.PrimaryHash("observation-alice")].Answer.Answer)
	assert.Equal(t, "bob answer", loaded.ObservationCache[common.PrimaryHash("observation-bob")].Answer.Answer)
}
