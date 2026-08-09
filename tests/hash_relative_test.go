package tests_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/stretchr/testify/assert"
)

// Observations are shared through git; identical repos checked out at
// different locations must produce identical cache identities.
func TestHashStableAcrossWorkspaceRoots(t *testing.T) {
	content := "package p\n\nfunc x(v int) string {\n\treturn string(v * 2)\n}\n"

	type identity struct {
		file     string
		module   string
		fn       string
		fnDebug  string
		position string
	}

	identities := make([]identity, 0, 2)

	for range 2 {
		workspace := t.TempDir()
		if !assert.NoError(t, os.WriteFile(filepath.Join(workspace, "mod.go"), []byte(content), 0o644)) {
			return
		}

		pf, err := cog.ParseFile(t.Context(), workspace, "mod.go")
		if !assert.NoError(t, err) {
			return
		}

		fn := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)
		identities = append(identities, identity{
			file:     pf.GetHash().String(),
			module:   ast.GetHash(pf.Module).String(),
			fn:       ast.GetHash(fn).String(),
			fnDebug:  fn.DebugPosition().SourceHash,
			position: fn.GetPosition().RelativeString(),
		})
	}

	assert.Equal(t, identities[0], identities[1])
	assert.Equal(t, "mod.go:3:1-5:2", identities[0].position)
}
