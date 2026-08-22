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

func TestCOGPHPFindTightestEnclosingNode(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/php/baseproj/method.php")

	rng, err := common.NewFileRangeAutoBytePosition(pf.Source, 13, 15, 13, 22)
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

	rng, err = common.NewFileRangeAutoBytePosition(pf.Source, 4, 4, 13, 22)
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

func TestCOGPHPImports(t *testing.T) {
	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	pf, err := cog.ParseFile(t.Context(), cwd, "./fixtures/php/multidep/main.php")
	if !assert.NoError(t, err) {
		return
	}

	// intelephense indexes the workspace in the background and answers
	// definitions empty until it lands, so retry until both use clauses
	// resolve; the require_once import never resolves and stays absent
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
		"DateTime":  false,
		"App\\Dep1": false,
	}
	types := map[string]common.DependencyType{
		"DateTime":  common.StandardLibraryDependency,
		"App\\Dep1": common.LocalDependency,
	}

	for k, deps := range packs {
		key := string(k.GetBytes())
		checkSet[key] = true
		for _, dep := range deps {
			assert.Equal(t, types[key], dep.Type, "unexpected dependency type for %v -> %v", key, dep.External.Source.Path)
		}
	}

	assert.Equal(t, map[string]bool{
		"DateTime":  true,
		"App\\Dep1": true,
	}, checkSet)
}

func TestPHPSearchSnippetRootsAtEnclosingFunction(t *testing.T) {
	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	wspace := cog.NewWorkspace(cwd)

	// intelephense indexes in the background; retry until the stdlib and
	// local edges both appear
	var root cog.COGNode
	edges := 0

	for attempt := 0; attempt < 10; attempt++ {
		og, r, err := wspace.SearchSnippet(t.Context(), "./fixtures/php/multidep/main.php",
			"$now = new DateTime();\n    return FixtureDep1::exampleText($now);")

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

	assert.GreaterOrEqual(t, edges, 2, "expected the stdlib and local dependency edges")
}

func TestPHPClassifyImportType(t *testing.T) {
	// Paths mirror what intelephense actually returns across platforms,
	// after filepath.ToSlash; both absolute and workspace-relative forms occur
	cases := map[string]common.DependencyType{
		"/Users/u/.nvm/versions/node/v26.7.0/lib/node_modules/intelephense/lib/stub/date/date_c.php":      common.StandardLibraryDependency,
		"../../.nvm/versions/node/v26.7.0/lib/node_modules/intelephense/lib/stub/standard/standard_1.php": common.StandardLibraryDependency,
		"C:/Users/u/AppData/Roaming/npm/node_modules/intelephense/lib/stub/Core/Core_c.php":               common.StandardLibraryDependency,
		"/srv/app/vendor/monolog/monolog/src/Monolog/Logger.php":                                          common.PackageDependency,
		"vendor/guzzlehttp/guzzle/src/Client.php":                                                         common.PackageDependency,
		"fixtures/php/multidep/dep1.php":                                                                  common.LocalDependency,
		"/srv/app/src/Service/Mailer.php":                                                                 common.LocalDependency,
	}
	for path, want := range cases {
		got := languages.PHP.ClassifyImportType(common.NewSource("", path, nil))
		assert.Equal(t, want, got, path)
	}
}

func TestPHPRequireLSPServer(t *testing.T) {
	ctx := t.Context()

	req := languages.PHP.GetLSPServerRequirement()
	assert.Equal(t, "intelephense", req.Name)
	assert.Equal(t, "npm install -g intelephense", req.InstallCommand)

	// Renamed to keep the process-wide located-server memo isolated
	req.Name = t.Name()

	bin := t.TempDir()
	t.Setenv("PATH", toolchainOnlyPath(t))

	err := cog.RequireLSPServer(ctx, req)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "intelephense")
		assert.Contains(t, err.Error(), req.InstallCommand)
		assert.ErrorIs(t, err, lsp.ErrServerMissing)
	}

	assert.NoError(t, os.WriteFile(filepath.Join(bin, "intelephense"), nil, 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+toolchainOnlyPath(t))

	assert.NoError(t, cog.RequireLSPServer(ctx, req))
}
