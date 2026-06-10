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
	checkSnapshot(t, "./fixtures/golang/multidep/cmd/main.go")
}

func TestCOGSimpleImportsSnap(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/golang/multidep/cmd/main.go")

	imps, err := pf.ListImports(t.Context())

	assert.NoError(t, err)

	packs := imps.GroupByPackage()
	checkSet := map[string]bool{
		"\"fmt\"": false,
		"fixture_dep1 \"github.com/schahriar/captn/tests/fixtures/golang/multidep/pkg/dep1\"": false,
	}

	for k := range packs {
		checkSet[string(k.GetBytes())] = true
	}

	assert.Equal(t, map[string]bool{
		"\"fmt\"": true,
		"fixture_dep1 \"github.com/schahriar/captn/tests/fixtures/golang/multidep/pkg/dep1\"": true,
	}, checkSet)
}
