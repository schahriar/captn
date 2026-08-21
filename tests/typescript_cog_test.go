package tests_test

import (
	"os"
	"testing"
	"time"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/common"
	"github.com/stretchr/testify/assert"
)

// tsAwaitDependencies retries ListDependencies until tsserver's configured
// project has loaded: before that it answers definitions from single-file
// syntax only and imports resolve to nothing.
func tsAwaitDependencies(t *testing.T, pf *cog.COGFile, groups int) common.ResolvedDependencies {
	t.Helper()

	var imps common.ResolvedDependencies

	for attempt := 0; attempt < 10; attempt++ {
		var err error
		imps, err = pf.ListDependencies(t.Context())

		if !assert.NoError(t, err) {
			return nil
		}

		if len(imps.GroupByPackage()) >= groups {
			break
		}

		time.Sleep(2 * time.Second)
	}

	return imps
}

func TestCOGTypescriptImports(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/typescript/multidep/main.ts")

	imps := tsAwaitDependencies(t, pf, 2)
	if imps == nil {
		return
	}

	packs := imps.GroupByPackage()

	checkSet := map[string]bool{
		`"node:fs"`:    false,
		`"./pkg/dep1"`: false,
	}
	types := map[string]common.DependencyType{
		`"node:fs"`:    common.StandardLibraryDependency,
		`"./pkg/dep1"`: common.LocalDependency,
	}

	for k, deps := range packs {
		key := string(k.GetBytes())
		checkSet[key] = true
		for _, dep := range deps {
			assert.Equal(t, types[key], dep.Type, "unexpected dependency type for %v -> %v", key, dep.External.Source.Path)
		}
	}

	assert.Equal(t, map[string]bool{
		`"node:fs"`:    true,
		`"./pkg/dep1"`: true,
	}, checkSet)
}

func TestTypescriptSearchSnippetRootsAtEnclosingFunction(t *testing.T) {
	cwd, err := os.Getwd()
	assert.NoError(t, err)

	wspace := cog.NewWorkspace(cwd)

	// Waiting on the project load keeps the definition batch below from
	// answering with pre-load local bindings
	pf, err := wspace.LoadFile(t.Context(), "./fixtures/typescript/multidep/main.ts")
	if !assert.NoError(t, err) {
		return
	}

	if tsAwaitDependencies(t, pf, 2) == nil {
		return
	}

	og, root, err := wspace.SearchSnippet(t.Context(), "./fixtures/typescript/multidep/main.ts", `readFileSync(fixtureDep1(), "utf8")`)

	if !assert.NoError(t, err) {
		return
	}

	fn, ok := root.(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected root to be the enclosing function, got %T", root) && assert.NotNil(t, fn.Name) {
		assert.Equal(t, "main", fn.Name.Name)
	}

	adj, err := og.Graph.AdjacencyMap()
	if !assert.NoError(t, err) {
		return
	}

	assert.NotContains(t, adj, pf.Module.GetHash())

	dep, err := wspace.LoadFile(t.Context(), "fixtures/typescript/multidep/pkg/dep1.ts")
	if !assert.NoError(t, err) {
		return
	}

	getExampleText := namedFunction(t, dep, "getExampleText")
	if getExampleText == nil {
		return
	}

	assert.Contains(t, adj, ast.GetHash(getExampleText), "expected the cross-file definition in the graph")
}

// A type used only in a signature must still pull its definition into the
// graph, and the `expected 1 node` hard error is what this guards
func TestTypescriptSearchSnippetResolvesSignatureTypes(t *testing.T) {
	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	wspace := cog.NewWorkspace(cwd)

	og, root, err := wspace.SearchSnippet(
		t.Context(),
		"./fixtures/typescript/baseproj/method.ts",
		"function label(w: Widget): string {",
	)

	if !assert.NoError(t, err) {
		return
	}

	fn, ok := root.(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected root to be the enclosing function, got %T", root) {
		return
	}

	assert.Equal(t, "label", fn.Name.Name)

	pf, err := wspace.LoadFile(t.Context(), "./fixtures/typescript/baseproj/method.ts")
	if !assert.NoError(t, err) {
		return
	}

	widget := namedFunction(t, pf, "Widget")
	if widget == nil {
		return
	}

	adj, err := og.Graph.AdjacencyMap()
	if !assert.NoError(t, err) {
		return
	}

	assert.Contains(t, adj, ast.GetHash(widget), "expected the parameter type in the graph")
}

// Every use of a type parameter resolves back onto its declaration in the
// signature, which must hold exactly one node or the search hard-errors
func TestTypescriptSearchSnippetResolvesTypeParameters(t *testing.T) {
	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	wspace := cog.NewWorkspace(cwd)

	_, root, err := wspace.SearchSnippet(t.Context(), "./fixtures/typescript/baseproj/generic.ts", "function mapValues<T, U>(s: T[], f: (v: T) => U): U[] {")
	if !assert.NoError(t, err) {
		return
	}

	fn, ok := root.(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected root to be the generic function, got %T", root) && assert.NotNil(t, fn.Name) {
		assert.Equal(t, "mapValues", fn.Name.Name)
	}
}

// A builtin call resolves into the typescript lib files, which answer with
// interfaces, an ambient var, and whole call and construct signatures; every
// one of those replies must land on exactly one node
func TestTypescriptSearchSnippetResolvesBuiltins(t *testing.T) {
	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	wspace := cog.NewWorkspace(cwd)

	_, root, err := wspace.SearchSnippet(t.Context(), "./fixtures/typescript/baseproj/simple.ts", "return String(v * 2);")
	if !assert.NoError(t, err) {
		return
	}

	fn, ok := root.(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected root to be the enclosing function, got %T", root) && assert.NotNil(t, fn.Name) {
		assert.Equal(t, "x", fn.Name.Name)
	}
}
