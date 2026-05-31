package tests_test

import (
	"os"
	"testing"

	"github.com/gkampitakis/go-snaps/snaps"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/stretchr/testify/assert"
)

func checkSnapshot(t *testing.T, path string) {
	t.Helper()

	ctx := t.Context()
	cwd, err := os.Getwd()

	assert.NoError(t, err)

	pf, err := cog.ParseFile(ctx, cwd, path)

	assert.NoError(t, err)

	snaps.MatchYAML(t, pf.Module.Debug())
}

func TestParserSimpleFuncParse(t *testing.T) {
	checkSnapshot(t, "./fixtures/golang/baseproj/simple.go")
}

func TestParserMultiDepParse(t *testing.T) {
	// TODO: Update with import deps
	checkSnapshot(t, "./fixtures/golang/multidep/cmd/main.go")
}
