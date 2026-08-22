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

func TestCOGCPPFindTightestEnclosingNode(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/cpp/baseproj/method.cpp")

	rng, err := pf.FindSnippetRange([]byte("return label_;"))
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

	rng, err = pf.FindSnippetRange([]byte("int area() const;\n\tstd::string describe"))
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

func listCDependencies(t *testing.T, path string, wantPacks int) map[*common.FileRange]common.ResolvedDependencies {
	t.Helper()

	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return nil
	}

	pf, err := cog.ParseFile(t.Context(), cwd, path)
	if !assert.NoError(t, err) {
		return nil
	}

	// clangd usually answers immediately, but retry while its index warms up
	var packs map[*common.FileRange]common.ResolvedDependencies

	for attempt := 0; attempt < 5; attempt++ {
		imps, err := pf.ListDependencies(t.Context())
		if !assert.NoError(t, err) {
			return nil
		}

		packs = imps.GroupByPackage()

		if len(packs) >= wantPacks {
			break
		}

		time.Sleep(2 * time.Second)
	}

	return packs
}

func TestCOGCImports(t *testing.T) {
	packs := listCDependencies(t, "./fixtures/c/multidep/main.c", 2)
	if packs == nil {
		return
	}

	checkSet := map[string]bool{
		"<string.h>": false,
		"\"widget.h\"": false,
	}
	types := map[string]common.DependencyType{
		"<string.h>": common.StandardLibraryDependency,
		"\"widget.h\"": common.LocalDependency,
	}

	for k, deps := range packs {
		key := string(k.GetBytes())
		checkSet[key] = true
		for _, dep := range deps {
			assert.Equal(t, types[key], dep.Type, "unexpected dependency type for %v -> %v", key, dep.External.Source.Path)
		}
	}

	assert.Equal(t, map[string]bool{
		"<string.h>": true,
		"\"widget.h\"": true,
	}, checkSet)
}

func TestCOGCPPImports(t *testing.T) {
	packs := listCDependencies(t, "./fixtures/cpp/multidep/main.cpp", 2)
	if packs == nil {
		return
	}

	checkSet := map[string]bool{
		"<string>": false,
		"\"gadget.hpp\"": false,
	}
	types := map[string]common.DependencyType{
		"<string>": common.StandardLibraryDependency,
		"\"gadget.hpp\"": common.LocalDependency,
	}

	for k, deps := range packs {
		key := string(k.GetBytes())
		checkSet[key] = true
		for _, dep := range deps {
			assert.Equal(t, types[key], dep.Type, "unexpected dependency type for %v -> %v", key, dep.External.Source.Path)
		}
	}

	assert.Equal(t, map[string]bool{
		"<string>": true,
		"\"gadget.hpp\"": true,
	}, checkSet)
}

func TestCSearchSnippetRootsAtEnclosingFunction(t *testing.T) {
	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	wspace := cog.NewWorkspace(cwd)

	og, root, err := wspace.SearchSnippet(t.Context(), "./fixtures/c/multidep/main.c",
		"Widget w = widget_make(\"card\");")

	if !assert.NoError(t, err) {
		return
	}

	fn, ok := root.(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected root to be the enclosing function, got %T", root) {
		assert.Equal(t, "run", fn.Name.Name)
	}

	adj, err := og.Graph.AdjacencyMap()
	if !assert.NoError(t, err) {
		return
	}

	edges := 0
	for _, out := range adj {
		edges += len(out)
	}

	// The type and the call both resolve into widget.h
	assert.GreaterOrEqual(t, edges, 2, "expected the Widget and widget_make edges")
}

func TestCPPSearchSnippetSignatureTypesOnly(t *testing.T) {
	// A type used only in a signature must still pull its definition into
	// the graph; this guards the `expected 1 node` hard error on template
	// parameters and aliases
	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	wspace := cog.NewWorkspace(cwd)

	og, root, err := wspace.SearchSnippet(t.Context(), "./fixtures/cpp/baseproj/types.cpp",
		"T last(const CardList &cards, T fallback)")

	if !assert.NoError(t, err) {
		return
	}

	fn, ok := root.(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected root to be the template function, got %T", root) {
		assert.Equal(t, "last", fn.Name.Name)
	}

	adj, err := og.Graph.AdjacencyMap()
	if !assert.NoError(t, err) {
		return
	}

	edges := 0
	for _, out := range adj {
		edges += len(out)
	}

	assert.GreaterOrEqual(t, edges, 1, "expected the CardList alias edge")
}

func TestCPPSearchSnippetMethodCalls(t *testing.T) {
	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	wspace := cog.NewWorkspace(cwd)

	og, root, err := wspace.SearchSnippet(t.Context(), "./fixtures/cpp/multidep/main.cpp",
		"std::string label = g.describe();\n\treturn static_cast<int>(label.size()) + gadgets::rank(g);")

	if !assert.NoError(t, err) {
		return
	}

	fn, ok := root.(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected root to be the enclosing function, got %T", root) {
		assert.Equal(t, "run", fn.Name.Name)
	}

	adj, err := og.Graph.AdjacencyMap()
	if !assert.NoError(t, err) {
		return
	}

	edges := 0
	for _, out := range adj {
		edges += len(out)
	}

	// describe and rank resolve into gadget.hpp; size resolves into libc++'s
	// extensionless <string> and is deliberately dropped
	assert.GreaterOrEqual(t, edges, 2, "expected the describe and rank edges")
}

func TestCClassifyImportType(t *testing.T) {
	// Paths mirror what clangd actually returns across platforms, after
	// filepath.ToSlash; both absolute and workspace-relative forms occur
	cases := map[string]common.DependencyType{
		"/Applications/Xcode.app/Contents/Developer/Platforms/MacOSX.platform/Developer/SDKs/MacOSX.sdk/usr/include/string.h": common.StandardLibraryDependency,
		"/Applications/Xcode.app/Contents/Developer/Platforms/MacOSX.platform/Developer/SDKs/MacOSX.sdk/usr/include/c++/v1/__vector/vector.h": common.StandardLibraryDependency,
		"/Library/Developer/CommandLineTools/usr/lib/clang/17/include/stddef.h":                                               common.StandardLibraryDependency,
		"../../../../Library/Developer/CommandLineTools/SDKs/MacOSX.sdk/usr/include/stdio.h":                                  common.StandardLibraryDependency,
		"/usr/include/c++/13/bits/basic_string.h":                                                                             common.StandardLibraryDependency,
		"/usr/lib/gcc/x86_64-linux-gnu/13/include/stddef.h":                                                                   common.StandardLibraryDependency,
		"C:/Program Files (x86)/Windows Kits/10/Include/10.0.22621.0/ucrt/string.h":                                           common.StandardLibraryDependency,
		"/opt/homebrew/include/curl/curl.h":                                                                                   common.PackageDependency,
		"/srv/app/vcpkg_installed/x64-linux/include/fmt/format.h":                                                             common.PackageDependency,
		"fixtures/c/multidep/widget.h":                                                                                        common.LocalDependency,
		"/srv/app/src/widget.h":                                                                                               common.LocalDependency,
	}
	for path, want := range cases {
		got := languages.C.ClassifyImportType(common.NewSource("", path, nil))
		assert.Equal(t, want, got, path)
	}
}

func TestCRequireLSPServer(t *testing.T) {
	ctx := t.Context()

	req := languages.C.GetLSPServerRequirement()
	assert.Equal(t, "clangd", req.Name)
	assert.NotEmpty(t, req.InstallCommand)

	// Renamed to keep the process-wide located-server memo isolated
	req.Name = t.Name()

	bin := t.TempDir()
	t.Setenv("PATH", toolchainOnlyPath(t))

	err := cog.RequireLSPServer(ctx, req)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), req.InstallCommand)
		assert.ErrorIs(t, err, lsp.ErrServerMissing)
	}

	assert.NoError(t, os.WriteFile(filepath.Join(bin, "clangd"), nil, 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+toolchainOnlyPath(t))

	assert.NoError(t, cog.RequireLSPServer(ctx, req))
}
