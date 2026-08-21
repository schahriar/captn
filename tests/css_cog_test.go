package tests_test

import (
	"os"
	"testing"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/languages"
	"github.com/stretchr/testify/assert"
)

// A var() use resolves onto the variable's declaration, pulling the rule
// that declares it into the graph
func TestCSSSearchSnippetResolvesVariable(t *testing.T) {
	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	wspace := cog.NewWorkspace(cwd)

	og, root, err := wspace.SearchSnippet(t.Context(), "./fixtures/css/baseproj/styles.css", "color: var(--accent);")
	if !assert.NoError(t, err) {
		return
	}

	button, ok := root.(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected root to be the rule, got %T", root) && assert.NotNil(t, button.Name) {
		assert.Equal(t, "button", button.Name.Name)
	}

	pf, err := wspace.LoadFile(t.Context(), "./fixtures/css/baseproj/styles.css")
	if !assert.NoError(t, err) {
		return
	}

	declaring := cssFirstRule(t, pf)
	if declaring == nil {
		return
	}

	adj, err := og.Graph.AdjacencyMap()
	if !assert.NoError(t, err) {
		return
	}

	assert.Contains(t, adj, ast.GetHash(declaring), "expected the declaring rule in the graph")
}

// The server answers definitions only inside the opened document, so a
// variable declared in another file resolves to nothing: the graph stays a
// single vertex rather than erroring. A canary for when the server grows up.
func TestCSSCrossFileVariableDegradesSilently(t *testing.T) {
	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	wspace := cog.NewWorkspace(cwd)

	og, root, err := wspace.SearchSnippet(t.Context(), "./fixtures/css/multidep/main.css", "background: var(--brand);")
	if !assert.NoError(t, err) {
		return
	}

	banner, ok := root.(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected root to be the rule, got %T", root) && assert.NotNil(t, banner.Name) {
		assert.Equal(t, "banner", banner.Name.Name)
	}

	adj, err := og.Graph.AdjacencyMap()
	if !assert.NoError(t, err) {
		return
	}

	assert.Len(t, adj, 1, "an out-of-file definition would be new server behavior")
}

// The css server answers no definition for @import, so the dependency
// listing degrades to empty rather than erroring. When a future server
// starts answering, this fails and classification needs tests in front of it.
func TestCSSListDependenciesDegradesSilently(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/css/multidep/main.css")

	imps, err := pf.ListDependencies(t.Context())

	assert.NoError(t, err)
	assert.Empty(t, imps, "an answering server would be new behavior; see the probe test")
}

// The install command is what captn hands the driving agent on a missing
// server; a typo here ships silently without this pin
func TestWebServerRequirements(t *testing.T) {
	for _, tc := range []struct {
		ls      languages.LanguageSupport
		name    string
		install string
	}{
		{languages.CSS, "vscode-css-language-server", "npm install -g vscode-langservers-extracted"},
		{languages.HTML, "vscode-html-language-server", "npm install -g vscode-langservers-extracted"},
		{languages.Typescript, "typescript-language-server", "npm install -g typescript-language-server typescript@5"},
	} {
		req := tc.ls.GetLSPServerRequirement()

		assert.Equal(t, tc.name, req.Name)
		assert.Equal(t, tc.install, req.InstallCommand)
		assert.NotNil(t, req.Locate)
	}
}
