package tests_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/languages"
	"github.com/schahriar/captn/pkg/lsp"
	"github.com/stretchr/testify/assert"
)

func TestCOGPythonRangeQueryFunction(t *testing.T) {
	pf := parsePySimple(t)

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

	full, err := common.NewFileRangeAutoBytePosition(pf.Source, 0, 0, 1, 21)
	if !assert.NoError(t, err) {
		return
	}

	kinds := common.Map(pf.FindNodesWithinRange(full), func(node ast.ASTNode) string {
		return node.Kind()
	})
	assert.Contains(t, kinds, "FuncExpression")
	assert.Contains(t, kinds, "FuncArgument")
	assert.Contains(t, kinds, "CallExpression")
	assert.NotContains(t, kinds, "Return")
}

func TestCOGPythonRangeQueryFindsMethodDefinitionSymbol(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/python/baseproj/method.py")

	rng, err := common.NewFileRangeAutoBytePosition(pf.Source, 1, 8, 1, 16)
	if !assert.NoError(t, err) {
		return
	}

	nodes := pf.FindNodesWithinRange(rng)
	if !assert.Len(t, nodes, 1) {
		return
	}
	sym, ok := nodes[0].(*ast.ASTSymbol)
	if assert.True(t, ok) {
		assert.Equal(t, "describe", sym.Name)
	}
}

func TestCOGPythonFindTightestEnclosingNode(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/python/baseproj/method.py")

	rng, err := common.NewFileRangeAutoBytePosition(pf.Source, 2, 15, 2, 20)
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

	rng, err = common.NewFileRangeAutoBytePosition(pf.Source, 0, 6, 2, 10)
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

func TestCOGPythonImports(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/python/multidep/main.py")

	imps, err := pf.ListDependencies(t.Context())
	assert.NoError(t, err)

	packs := imps.GroupByPackage()

	checkSet := map[string]bool{
		"json":                     false,
		"pkg.dep1 as fixture_dep1": false,
	}
	// pyright resolves stdlib into its bundled typeshed stubs and, when an
	// interpreter is discoverable, the interpreter sources too; both must
	// classify as stdlib
	types := map[string]common.DependencyType{
		"json":                     common.StandardLibraryDependency,
		"pkg.dep1 as fixture_dep1": common.LocalDependency,
	}

	for k, deps := range packs {
		key := string(k.GetBytes())
		checkSet[key] = true
		for _, dep := range deps {
			assert.Equal(t, types[key], dep.Type, "unexpected dependency type for %v -> %v", key, dep.External.Source.Path)
		}
	}

	assert.Equal(t, map[string]bool{
		"json":                     true,
		"pkg.dep1 as fixture_dep1": true,
	}, checkSet)
}

func TestPythonSearchSnippetRootsAtEnclosingFunction(t *testing.T) {
	cwd, err := os.Getwd()
	assert.NoError(t, err)

	wspace := cog.NewWorkspace(cwd)

	og, root, err := wspace.SearchSnippet(t.Context(), "./fixtures/python/multidep/main.py", "print(json.dumps(fixture_dep1.get_example_text()))")

	if !assert.NoError(t, err) {
		return
	}

	_, ok := root.(*ast.ASTFuncExpression)
	assert.True(t, ok, "expected root to be the enclosing function, got %T", root)

	f, err := wspace.LoadFile(t.Context(), "./fixtures/python/multidep/main.py")
	assert.NoError(t, err)

	adj, err := og.Graph.AdjacencyMap()
	assert.NoError(t, err)
	assert.NotContains(t, adj, f.Module.GetHash())
}

func TestPythonClassifyImportType(t *testing.T) {
	// Paths mirror what pyright actually returns across platforms, after
	// filepath.ToSlash; both absolute and workspace-relative forms occur
	cases := map[string]common.DependencyType{
		"../../.nvm/node_modules/pyright/dist/typeshed-fallback/stdlib/json/__init__.pyi":                     common.StandardLibraryDependency,
		"/x/pyright/dist/typeshed-fallback/stubs/requests/requests/__init__.pyi":                              common.PackageDependency,
		"/opt/homebrew/anaconda3/lib/python3.13/json/__init__.py":                                             common.StandardLibraryDependency,
		"/usr/lib/python3.12/os.py":                                                                           common.StandardLibraryDependency,
		"/home/u/.venv/lib/python3.12/site-packages/requests/__init__.py":                                     common.PackageDependency,
		"C:/Users/u/venv/Lib/site-packages/requests/__init__.py":                                              common.PackageDependency,
		"C:/Program Files/Python313/Lib/json/__init__.py":                                                     common.StandardLibraryDependency,
		"C:/Users/u/AppData/Local/Python/pythoncore-3.14-64/Lib/json/__init__.py":                             common.StandardLibraryDependency,
		"C:/Users/u/AppData/Roaming/uv/data/python/cpython-3.13.1-windows-x86_64-none/Lib/json/__init__.py":   common.StandardLibraryDependency,
		"C:/Users/u/.pyenv/pyenv-win/versions/3.10.5/Lib/json/__init__.py":                                    common.StandardLibraryDependency,
		"C:/Program Files/WindowsApps/PythonSoftwareFoundation.Python.3.12_3.12.2544.0_x64__q/Lib/json/__init__.py": common.StandardLibraryDependency,
		"C:/Users/u/anaconda3/Lib/json/__init__.py":                                                           common.StandardLibraryDependency,
		"fixtures/python/multidep/pkg/dep1.py":                                                                common.LocalDependency,
	}
	for path, want := range cases {
		got := languages.Python.ClassifyImportType(common.NewSource("", path, nil))
		assert.Equal(t, want, got, path)
	}
}

func TestPythonRequireLSPServer(t *testing.T) {
	ctx := t.Context()

	req := languages.Python.GetLSPServerRequirement()
	assert.Equal(t, "pyright-langserver", req.Name)
	assert.Equal(t, "npm install -g pyright", req.InstallCommand)

	// Renamed to keep the process-wide located-server memo isolated
	req.Name = t.Name()

	bin := t.TempDir()
	t.Setenv("PATH", toolchainOnlyPath(t))

	err := cog.RequireLSPServer(ctx, req)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "pyright")
		assert.Contains(t, err.Error(), req.InstallCommand)
		assert.ErrorIs(t, err, lsp.ErrServerMissing)
	}

	assert.NoError(t, os.WriteFile(filepath.Join(bin, "pyright-langserver"), nil, 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+toolchainOnlyPath(t))

	assert.NoError(t, cog.RequireLSPServer(ctx, req))
}
