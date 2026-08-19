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

func TestCOGSwiftRangeQueryFunction(t *testing.T) {
	pf := parseSwiftSimple(t)

	rng, err := common.NewFileRangeAutoBytePosition(pf.Source, 0, 5, 0, 6)
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

	full, err := common.NewFileRangeAutoBytePosition(pf.Source, 0, 0, 2, 1)
	if !assert.NoError(t, err) {
		return
	}

	kinds := common.Map(pf.FindNodesWithinRange(full), func(node ast.ASTNode) string {
		return node.Kind()
	})

	assert.Contains(t, kinds, "FuncExpression")
	assert.Contains(t, kinds, "FuncArgument")
	assert.Contains(t, kinds, "CallExpression")
	// Swift's return is a control_transfer_statement and is deliberately unmapped
	assert.NotContains(t, kinds, "Return")
}

func TestCOGSwiftRangeQueryFindsMethodDefinitionSymbol(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/swift/baseproj/Sources/BaseProj/method.swift")

	rng, err := common.NewFileRangeAutoBytePosition(pf.Source, 1, 9, 1, 17)
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

func TestCOGSwiftFindTightestEnclosingNode(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/swift/baseproj/Sources/BaseProj/method.swift")

	rng, err := common.NewFileRangeAutoBytePosition(pf.Source, 2, 8, 2, 14)
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

	rng, err = common.NewFileRangeAutoBytePosition(pf.Source, 0, 7, 1, 8)
	if !assert.NoError(t, err) {
		return
	}

	node = pf.FindTightestEnclosingNode(rng, cog.IsNodeOfInterest)
	if !assert.NotNil(t, node) {
		return
	}

	widget, ok := node.(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected the struct to be the tightest enclosing vertex") {
		assert.Equal(t, "Widget", widget.Name.Name)
	}
}

func TestSwiftClassifyImportType(t *testing.T) {
	// Paths mirror what sourcekit-lsp actually returns. Every definition that
	// leaves the current module lands in a generated interface under a
	// temporary directory, including the ones for local targets.
	cases := map[string]common.DependencyType{
		"/var/folders/1w/xx/T/sourcekit-lsp/GeneratedInterfaces/Swift.Misc.swiftinterface":                                                                     common.StandardLibraryDependency,
		"/var/folders/1w/xx/T/sourcekit-lsp/GeneratedInterfaces/Swift.String.swiftinterface":                                                                   common.StandardLibraryDependency,
		"/var/folders/1w/xx/T/sourcekit-lsp/GeneratedInterfaces/Foundation.swiftinterface":                                                                     common.PackageDependency,
		"/var/folders/1w/xx/T/sourcekit-lsp/GeneratedInterfaces/Dep1.swiftinterface":                                                                           common.PackageDependency,
		"/Applications/Xcode.app/Contents/Developer/Toolchains/XcodeDefault.xctoolchain/usr/lib/swift/FoundationEssentials.swiftmodule/arm64e.swiftinterface":  common.StandardLibraryDependency,
		"/Applications/Xcode.app/Contents/Developer/Platforms/MacOSX.platform/Developer/SDKs/MacOSX.sdk/usr/lib/swift/Swift.swiftmodule/arm64e.swiftinterface": common.StandardLibraryDependency,
		"/usr/lib/swift/linux/x86_64/Foundation.swiftmodule":                                                                                                   common.StandardLibraryDependency,
		"/home/u/.swiftly/toolchains/6.2.0/usr/lib/swift/linux/Swift.swiftinterface":                                                                           common.StandardLibraryDependency,
		"/w/app/.build/checkouts/swift-argument-parser/Sources/ArgumentParser/Parsing.swift":                                                                   common.PackageDependency,
		"/Users/u/Library/Developer/Xcode/DerivedData/App-abc/SourcePackages/checkouts/swift-log/Sources/Logging/Logger.swift":                                 common.PackageDependency,
		"fixtures/swift/multidep/Sources/Dep1/dep1.swift":                                                                                                      common.LocalDependency,
		"/w/app/Sources/App/main.swift": common.LocalDependency,
	}

	for path, want := range cases {
		got := languages.Swift.ClassifyImportType(common.NewSource("", path, nil))
		assert.Equal(t, want, got, path)
	}
}

func TestSwiftRequireLSPServer(t *testing.T) {
	ctx := t.Context()

	req := languages.Swift.GetLSPServerRequirement()
	assert.Equal(t, "sourcekit-lsp", req.Name)
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

	assert.NoError(t, os.WriteFile(filepath.Join(bin, "sourcekit-lsp"), nil, 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+toolchainOnlyPath(t))

	assert.NoError(t, cog.RequireLSPServer(ctx, req))
}

func TestCOGSwiftImports(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/swift/multidep/Sources/App/main.swift")

	imps, err := pf.ListDependencies(t.Context())
	if !assert.NoError(t, err) {
		return
	}

	packs := imps.GroupByPackage()

	checkSet := map[string]bool{
		"Foundation": false,
		"Dep1":       false,
	}

	// Swift resolves an import to its module's generated interface, never to a
	// source file, so even a local target reads as a module boundary here
	types := map[string]common.DependencyType{
		"Foundation": common.PackageDependency,
		"Dep1":       common.PackageDependency,
	}

	for k, deps := range packs {
		key := string(k.GetBytes())
		checkSet[key] = true

		for _, dep := range deps {
			assert.Equal(t, types[key], dep.Type, "unexpected dependency type for %v -> %v", key, dep.External.Source.Path)
		}
	}

	assert.Equal(t, map[string]bool{
		"Foundation": true,
		"Dep1":       true,
	}, checkSet)
}

func TestSwiftSearchSnippetRootsAtEnclosingFunction(t *testing.T) {
	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	wspace := cog.NewWorkspace(cwd)

	og, root, err := wspace.SearchSnippet(
		t.Context(),
		"./fixtures/swift/multidep/Sources/App/main.swift",
		"print(getExampleText().uppercased())",
	)

	if !assert.NoError(t, err) {
		return
	}

	fn, ok := root.(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected root to be the enclosing function, got %T", root) {
		return
	}

	assert.Equal(t, "main", fn.Name.Name)

	f, err := wspace.LoadFile(t.Context(), "./fixtures/swift/multidep/Sources/App/main.swift")
	if !assert.NoError(t, err) {
		return
	}

	adj, err := og.Graph.AdjacencyMap()
	assert.NoError(t, err)
	assert.NotContains(t, adj, f.Module.GetHash())
}
