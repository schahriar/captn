package tests_test

import (
	"os"
	"testing"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/languages"
	"github.com/stretchr/testify/assert"
)

// The html server answers no definitions; a search must still root and
// return without error
func TestHTMLSearchSnippetRootsAtElement(t *testing.T) {
	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	wspace := cog.NewWorkspace(cwd)

	_, root, err := wspace.SearchSnippet(t.Context(), "./fixtures/html/baseproj/index.html", `<div class="button" id="cta">`)
	if !assert.NoError(t, err) {
		return
	}

	fn, ok := root.(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected root to be the element, got %T", root) && assert.NotNil(t, fn.Name) {
		assert.Equal(t, "div", fn.Name.Name)
	}
}

// TestHTMLLSPProbe reports what vscode-html-language-server answers for
// definitions; HTML has no definitions to speak of, so an empty probe is the
// expected baseline rather than a failure.
func TestHTMLLSPProbe(t *testing.T) {
	if os.Getenv("CAPTN_PROBE") == "" {
		t.Skip("set CAPTN_PROBE=1 to probe vscode-html-language-server")
	}

	probeFileWith(t, languages.HTML, "./fixtures/html/baseproj/index.html")
}
