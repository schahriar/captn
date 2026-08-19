package tests_test

import (
	"testing"

	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/common"
	"github.com/stretchr/testify/assert"
)

// An empty file has a module that spans no bytes. Indexing one used to panic
// out of ParseSource in every language, which meant a repository holding so
// much as an empty __init__.py could not be read.
func TestParseEmptySourceEveryLanguage(t *testing.T) {
	for _, name := range []string{"empty.go", "empty.py", "empty.swift"} {
		cwd := t.TempDir()

		var pf *cog.COGFile
		var err error

		if !assert.NotPanics(t, func() {
			pf, err = cog.ParseSource(t.Context(), common.NewSource(cwd, name, []byte("")))
		}, "parsing %v must not panic", name) {
			continue
		}

		if !assert.NoError(t, err, "parsing %v", name) {
			continue
		}

		if !assert.NotNil(t, pf, "parsing %v", name) {
			continue
		}

		assert.NotNil(t, pf.Module, "%v should still carry a module", name)

		// A range query over the empty file answers with nothing rather than
		// failing
		assert.NotPanics(t, func() {
			pf.FindNodesWithinRange(pf.GetFileRange())
			pf.FindTightestEnclosingNode(pf.GetFileRange(), cog.IsNodeOfInterest)
		}, "querying %v must not panic", name)
	}
}

// Whitespace-only files reach the same code path with a non-empty buffer but
// still produce nodes that span nothing
func TestParseWhitespaceOnlySource(t *testing.T) {
	for _, name := range []string{"blank.go", "blank.py", "blank.swift"} {
		cwd := t.TempDir()

		assert.NotPanics(t, func() {
			if _, err := cog.ParseSource(t.Context(), common.NewSource(cwd, name, []byte("\n\n   \n"))); err != nil {
				assert.NoError(t, err, "parsing %v", name)
			}
		}, "parsing %v must not panic", name)
	}
}
