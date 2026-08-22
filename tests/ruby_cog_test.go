package tests_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/languages"
	"github.com/schahriar/captn/pkg/lsp"
	"github.com/stretchr/testify/assert"
)

// rubyWorkspace anchors a fixture project as its own workspace root:
// ruby-lsp's indexer hard-excludes **/fixtures/**, so definitions only
// resolve when the fixture directory itself is the workspace
func rubyWorkspace(t *testing.T, project string) string {
	t.Helper()
	cwd, err := os.Getwd()
	assert.NoError(t, err)
	return filepath.Join(cwd, "fixtures", "ruby", project)
}

func TestCOGRubyRangeQueryFunction(t *testing.T) {
	pf := parseRbSimple(t)

	rng, err := common.NewFileRangeAutoBytePosition(pf.Source, 0, 4, 0, 5)
	if !assert.NoError(t, err) {
		return
	}

	nodes := pf.FindNodesWithinRange(rng)
	if !assert.Len(t, nodes, 1) {
		return
	}
	sym, ok := nodes[0].(*ast.ASTSymbol)
	if assert.True(t, ok) {
		assert.Equal(t, "x", sym.Name)
	}
}

func TestCOGRubyFindTightestEnclosingNode(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/ruby/baseproj/method.rb")

	rng, err := common.NewFileRangeAutoBytePosition(pf.Source, 8, 4, 8, 10)
	if !assert.NoError(t, err) {
		return
	}

	node := pf.FindTightestEnclosingNode(rng, cog.IsNodeOfInterest)
	if !assert.NotNil(t, node) {
		return
	}
	fn, ok := node.(*ast.ASTFuncExpression)
	if assert.True(t, ok) {
		assert.Equal(t, "describe", fn.Name.Name)
	}

	rng, err = common.NewFileRangeAutoBytePosition(pf.Source, 1, 2, 8, 10)
	if !assert.NoError(t, err) {
		return
	}

	node = pf.FindTightestEnclosingNode(rng, cog.IsNodeOfInterest)
	if !assert.NotNil(t, node) {
		return
	}
	class, ok := node.(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected the class to be the tightest enclosing vertex") {
		assert.Equal(t, "Widget", class.Name.Name)
	}
}

func TestCOGRubyImports(t *testing.T) {
	workspace := rubyWorkspace(t, "multidep")
	pf, err := cog.ParseFile(t.Context(), workspace, "main.rb")
	if !assert.NoError(t, err) {
		return
	}

	// ruby-lsp resolves a stdlib require only once its background index
	// lands, so retry until both imports answer
	var packs map[*common.FileRange]common.ResolvedDependencies

	for attempt := 0; attempt < 10; attempt++ {
		imps, err := pf.ListDependencies(t.Context())
		if !assert.NoError(t, err) {
			return
		}

		packs = imps.GroupByPackage()

		if len(packs) >= 2 {
			break
		}

		time.Sleep(2 * time.Second)
	}

	checkSet := map[string]bool{
		"json": false,
		"dep1": false,
	}
	types := map[string]common.DependencyType{
		"json": common.StandardLibraryDependency,
		"dep1": common.LocalDependency,
	}

	for k, deps := range packs {
		key := string(k.GetBytes())
		checkSet[key] = true
		for _, dep := range deps {
			assert.Equal(t, types[key], dep.Type, "unexpected dependency type for %v -> %v", key, dep.External.Source.Path)
		}
	}

	assert.Equal(t, map[string]bool{
		"json": true,
		"dep1": true,
	}, checkSet)
}

func TestRubySearchSnippetRootsAtEnclosingFunction(t *testing.T) {
	wspace := cog.NewWorkspace(rubyWorkspace(t, "multidep"))

	// ruby-lsp indexes the workspace in the background after initialize and
	// answers definitions empty until it lands, so retry until the local
	// dependency edge appears
	var root cog.COGNode
	edges := 0

	for attempt := 0; attempt < 10; attempt++ {
		og, r, err := wspace.SearchSnippet(t.Context(), "main.rb", "puts JSON.generate(Dep1.example_text)")

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
	if assert.True(t, ok, "expected root to be the enclosing function, got %T", root) {
		assert.Equal(t, "run", fn.Name.Name)
	}

	// One edge per resolved callee: JSON.generate into the stdlib and
	// Dep1.example_text into the local module
	assert.GreaterOrEqual(t, edges, 2, "expected the stdlib and local dependency edges")
}

func TestRubyClassifyImportType(t *testing.T) {
	// Paths mirror what ruby-lsp actually returns across platforms, after
	// filepath.ToSlash; both absolute and workspace-relative forms occur
	cases := map[string]common.DependencyType{
		"/opt/homebrew/Cellar/ruby/4.0.6_1/lib/ruby/4.0.0/json.rb":                         common.StandardLibraryDependency,
		"/opt/homebrew/Cellar/ruby/4.0.6_1/lib/ruby/4.0.0/json/common.rb":                  common.StandardLibraryDependency,
		"/Users/u/.rbenv/versions/3.3.5/lib/ruby/3.3.0/json.rb":                            common.StandardLibraryDependency,
		"/usr/lib/ruby/2.6.0/json.rb":                                                      common.StandardLibraryDependency,
		"C:/Ruby32-x64/lib/ruby/3.2.0/json.rb":                                             common.StandardLibraryDependency,
		"/opt/homebrew/lib/ruby/gems/4.0.0/gems/rdoc-7.0.4/lib/rdoc/markup/heading.rb":     common.PackageDependency,
		"/Users/u/.rbenv/versions/3.3.5/lib/ruby/gems/3.3.0/gems/rails-7.1.0/lib/rails.rb": common.PackageDependency,
		"C:/Ruby32-x64/lib/ruby/gems/3.2.0/gems/rake-13.0.6/lib/rake.rb":                   common.PackageDependency,
		"../app/vendor/bundle/ruby/3.3.0/gems/sinatra-4.0.0/lib/sinatra.rb":                common.PackageDependency,
		"fixtures/ruby/multidep/dep1.rb":                                                   common.LocalDependency,
	}
	for path, want := range cases {
		got := languages.Ruby.ClassifyImportType(common.NewSource("", path, nil))
		assert.Equal(t, want, got, path)
	}
}

func TestRubyRequireLSPServer(t *testing.T) {
	ctx := t.Context()

	req := languages.Ruby.GetLSPServerRequirement()
	assert.Equal(t, "ruby-lsp", req.Name)
	assert.Equal(t, "gem install ruby-lsp", req.InstallCommand)

	// Renamed to keep the process-wide located-server memo isolated
	req.Name = t.Name()

	bin := t.TempDir()
	t.Setenv("PATH", toolchainOnlyPath(t))

	err := cog.RequireLSPServer(ctx, req)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "ruby-lsp")
		assert.Contains(t, err.Error(), req.InstallCommand)
		assert.ErrorIs(t, err, lsp.ErrServerMissing)
	}

	assert.NoError(t, os.WriteFile(filepath.Join(bin, "ruby-lsp"), nil, 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+toolchainOnlyPath(t))

	assert.NoError(t, cog.RequireLSPServer(ctx, req))
}
