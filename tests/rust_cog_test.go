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

func TestCOGRustImports(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/rust/multidep/src/main.rs")

	// rust-analyzer answers definitions empty until cargo metadata loads, so
	// retry until every import resolves
	var packs map[*common.FileRange]common.ResolvedDependencies

	for attempt := 0; attempt < 10; attempt++ {
		imps, err := pf.ListDependencies(t.Context())
		if !assert.NoError(t, err) {
			return
		}

		packs = imps.GroupByPackage()

		if len(packs) >= 3 {
			break
		}

		time.Sleep(2 * time.Second)
	}

	checkSet := map[string]bool{
		"HashMap":          false,
		"dep1":             false,
		"get_example_text": false,
	}
	types := map[string]common.DependencyType{
		"HashMap":          common.StandardLibraryDependency,
		"dep1":             common.LocalDependency,
		"get_example_text": common.LocalDependency,
	}

	for k, deps := range packs {
		key := string(k.GetBytes())
		checkSet[key] = true
		for _, dep := range deps {
			assert.Equal(t, types[key], dep.Type, "unexpected dependency type for %v -> %v", key, dep.External.Source.Path)
		}
	}

	assert.Equal(t, map[string]bool{
		"HashMap":          true,
		"dep1":             true,
		"get_example_text": true,
	}, checkSet)
}

func TestRustSearchSnippetRootsAtEnclosingFunction(t *testing.T) {
	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	wspace := cog.NewWorkspace(cwd)

	// The insert edge pulls std's hash map source into the graph, which also
	// proves captn parses a full standard library file
	var root cog.COGNode
	edges := 0

	for attempt := 0; attempt < 10; attempt++ {
		og, r, err := wspace.SearchSnippet(t.Context(), "./fixtures/rust/multidep/src/main.rs", "seen.insert(example_text(), 1)")

		if !assert.NoError(t, err) {
			return
		}

		root = r

		adj, err := og.Graph.AdjacencyMap()
		if !assert.NoError(t, err) {
			return
		}

		edges = 0
		for _, out := range adj {
			edges += len(out)
		}

		if edges >= 2 {
			break
		}

		time.Sleep(2 * time.Second)
	}

	fn, ok := root.(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected root to be the enclosing function, got %T", root) && assert.NotNil(t, fn.Name) {
		assert.Equal(t, "main", fn.Name.Name)
	}

	// One edge per resolved callee: insert into the stdlib and example_text
	// into the local module
	assert.GreaterOrEqual(t, edges, 2, "expected the stdlib and local dependency edges")
}

// A type used only in a signature must still pull its definition into the graph
func TestRustSearchSnippetResolvesSignatureTypes(t *testing.T) {
	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	wspace := cog.NewWorkspace(cwd)

	pf := parseTestFile(t, "./fixtures/rust/baseproj/src/traits.rs")

	store := namedFunction(t, pf, "Store")
	if store == nil {
		return
	}

	var root cog.COGNode
	found := false

	for attempt := 0; attempt < 10; attempt++ {
		og, r, err := wspace.SearchSnippet(t.Context(), "./fixtures/rust/baseproj/src/traits.rs", "pub fn use_store(s: &dyn Store) -> String {")

		if !assert.NoError(t, err) {
			return
		}

		root = r

		adj, err := og.Graph.AdjacencyMap()
		if !assert.NoError(t, err) {
			return
		}

		if _, found = adj[ast.GetHash(store)]; found {
			break
		}

		time.Sleep(2 * time.Second)
	}

	fn, ok := root.(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected root to be the enclosing function, got %T", root) && assert.NotNil(t, fn.Name) {
		assert.Equal(t, "use_store", fn.Name.Name)
	}

	assert.True(t, found, "expected the trait definition in the graph")
}

// A call through a trait resolves onto the requirement, which is its own
// vertex under the trait
func TestRustSearchSnippetResolvesTraitRequirement(t *testing.T) {
	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	wspace := cog.NewWorkspace(cwd)

	pf := parseTestFile(t, "./fixtures/rust/baseproj/src/traits.rs")

	store := namedFunction(t, pf, "Store")
	if store == nil || !assert.Len(t, store.Block.Children(), 1) {
		return
	}

	get, ok := store.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected the requirement, got %T", store.Block.Children()[0]) {
		return
	}

	found := false

	for attempt := 0; attempt < 10; attempt++ {
		og, _, err := wspace.SearchSnippet(t.Context(), "./fixtures/rust/baseproj/src/traits.rs", `s.get("x")`)

		if !assert.NoError(t, err) {
			return
		}

		adj, err := og.Graph.AdjacencyMap()
		if !assert.NoError(t, err) {
			return
		}

		if _, found = adj[ast.GetHash(get)]; found {
			break
		}

		time.Sleep(2 * time.Second)
	}

	assert.True(t, found, "expected the trait requirement in the graph")
}
