package tests_test

import (
	"os"
	"testing"

	"github.com/gkampitakis/go-snaps/snaps"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/stretchr/testify/assert"
)

func TestParserSimpleFuncParse(t *testing.T) {
	ctx := t.Context()
	cwd, err := os.Getwd()

	assert.NoError(t, err)

	pf, err := cog.ParseFile(ctx, cwd, "./fixtures/baseproj/simple.go")

	assert.NoError(t, err)

	snaps.MatchYAML(t, pf.Module.Debug())
}
