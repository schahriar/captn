package tests_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/languages"
	"github.com/stretchr/testify/assert"
)

func parseInline(t *testing.T, name string, source string) *cog.COGFile {
	t.Helper()

	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return nil
	}

	pf, err := cog.ParseSource(t.Context(), common.NewSource(cwd, name, []byte(source)))
	if !assert.NoError(t, err, "expected %q to parse", source) {
		return nil
	}

	return pf
}

func firstFunction(t *testing.T, pf *cog.COGFile) *ast.ASTFuncExpression {
	t.Helper()

	for _, chld := range pf.Module.Block.Children() {
		if fn, ok := chld.(*ast.ASTFuncExpression); ok {
			return fn
		}
	}

	assert.Fail(t, "expected a function in the module block")
	return nil
}

func assertTypeSource(t *testing.T, texpr *ast.ASTTypeExpression, want string) {
	t.Helper()

	if !assert.NotNil(t, texpr) {
		return
	}

	assert.Equal(t, want, texpr.Name)
	assert.Equal(t, want, texpr.GetStringSource(), "type expression must span the identifier alone")
}

func TestTypeExpressionGolang(t *testing.T) {
	pf := parseInline(t, "types.go", "package p\n\nfunc f(a Widget, b []Widget, c map[string]Widget, d pkg.Thing) Report {\n\treturn Report{}\n}\n")
	if pf == nil {
		return
	}

	fn := firstFunction(t, pf)
	if !assert.Len(t, fn.Arguments, 4) {
		return
	}

	assertTypeSource(t, fn.Arguments[0].Type, "Widget")
	assertTypeSource(t, fn.Arguments[1].Type, "Widget")

	// map[string]Widget has no identifier of its own; the key leads and the
	// value hangs off it
	if assertTypeSource(t, fn.Arguments[2].Type, "string"); fn.Arguments[2].Type != nil {
		if assert.Len(t, fn.Arguments[2].Type.Arguments, 1) {
			assertTypeSource(t, fn.Arguments[2].Type.Arguments[0], "Widget")
		}
	}

	if assertTypeSource(t, fn.Arguments[3].Type, "Thing"); fn.Arguments[3].Type != nil {
		if assert.NotNil(t, fn.Arguments[3].Type.Namespace) {
			assert.Equal(t, "pkg", fn.Arguments[3].Type.Namespace.Name)
		}
	}

	assertTypeSource(t, fn.ReturnType, "Report")
}

func TestTypeExpressionGolangGeneric(t *testing.T) {
	pf := parseInline(t, "generic.go", "package p\n\nfunc f(a Result[Widget]) Widget {\n\treturn a.v\n}\n")
	if pf == nil {
		return
	}

	fn := firstFunction(t, pf)
	if !assert.Len(t, fn.Arguments, 1) {
		return
	}

	if assertTypeSource(t, fn.Arguments[0].Type, "Result"); fn.Arguments[0].Type != nil {
		if assert.Len(t, fn.Arguments[0].Type.Arguments, 1) {
			assertTypeSource(t, fn.Arguments[0].Type.Arguments[0], "Widget")
		}
	}
}

func TestTypeExpressionGolangDeclaresNamedTypes(t *testing.T) {
	// A resolved type reference needs a node at its definition; without this
	// SearchSnippet fails with "expected 1 node for dependent node"
	pf := parseInline(t, "decl.go", "package p\n\ntype Widget struct{}\n\ntype Alias = Widget\n\nfunc f[T any](v T) {}\n")
	if pf == nil {
		return
	}

	names := declaredNames(pf.Module)
	assert.Contains(t, names, "Widget")
	assert.Contains(t, names, "Alias")

	// A type parameter is an argument of the declaring function, not a
	// declaration in its block
	assert.NotContains(t, names, "T")

	for _, chld := range pf.Module.Block.Children() {
		fn, ok := chld.(*ast.ASTFuncExpression)
		if !ok || fn.Name == nil || fn.Name.Name != "f" {
			continue
		}

		if assert.Len(t, fn.Arguments, 2) && assert.NotNil(t, fn.Arguments[0].Identifier) {
			assert.Equal(t, "T", fn.Arguments[0].Identifier.Name)
			assertTypeSource(t, fn.Arguments[0].Type, "any")
		}
	}
}

func TestTypeExpressionGolangVarDeclaration(t *testing.T) {
	pf := parseInline(t, "vars.go", "package p\n\nvar w Widget\n")
	if pf == nil {
		return
	}

	decl, ok := pf.Module.Block.Children()[0].(*ast.ASTDeclaration)
	if !assert.True(t, ok, "expected ASTDeclaration for the var") {
		return
	}

	assertTypeSource(t, decl.Type, "Widget")
}

func TestTypeExpressionPython(t *testing.T) {
	pf := parseInline(t, "types.py", "def f(a: Widget, b: List[Widget], c: pkg.Thing) -> Report:\n    return a\n")
	if pf == nil {
		return
	}

	fn := firstFunction(t, pf)
	if !assert.Len(t, fn.Arguments, 3) {
		return
	}

	assertTypeSource(t, fn.Arguments[0].Type, "Widget")

	if assertTypeSource(t, fn.Arguments[1].Type, "List"); fn.Arguments[1].Type != nil {
		if assert.Len(t, fn.Arguments[1].Type.Arguments, 1) {
			assertTypeSource(t, fn.Arguments[1].Type.Arguments[0], "Widget")
		}
	}

	if assertTypeSource(t, fn.Arguments[2].Type, "Thing"); fn.Arguments[2].Type != nil {
		if assert.NotNil(t, fn.Arguments[2].Type.Namespace) {
			assert.Equal(t, "pkg", fn.Arguments[2].Type.Namespace.Name)
		}
	}

	assertTypeSource(t, fn.ReturnType, "Report")
}

func TestTypeExpressionPythonAnnotatedAssignment(t *testing.T) {
	pf := parseInline(t, "ann.py", "w: Widget = make()\n")
	if pf == nil {
		return
	}

	decl, ok := pf.Module.Block.Children()[0].(*ast.ASTDeclaration)
	if !assert.True(t, ok, "expected ASTDeclaration for the annotated assignment") {
		return
	}

	assertTypeSource(t, decl.Type, "Widget")
}

func TestTypeExpressionSwift(t *testing.T) {
	pf := parseInline(t, "types.swift", "func f(a: Widget, b: [Widget], c: Foo.Bar, d: Array<Widget>) -> Report {\n    return a\n}\n")
	if pf == nil {
		return
	}

	fn := firstFunction(t, pf)
	if !assert.Len(t, fn.Arguments, 4) {
		return
	}

	assertTypeSource(t, fn.Arguments[0].Type, "Widget")
	assertTypeSource(t, fn.Arguments[1].Type, "Widget")

	if assertTypeSource(t, fn.Arguments[2].Type, "Bar"); fn.Arguments[2].Type != nil {
		if assert.NotNil(t, fn.Arguments[2].Type.Namespace) {
			assert.Equal(t, "Foo", fn.Arguments[2].Type.Namespace.Name)
		}
	}

	if assertTypeSource(t, fn.Arguments[3].Type, "Array"); fn.Arguments[3].Type != nil {
		if assert.Len(t, fn.Arguments[3].Type.Arguments, 1) {
			assertTypeSource(t, fn.Arguments[3].Type.Arguments[0], "Widget")
		}
	}

	assertTypeSource(t, fn.ReturnType, "Report")
}

// A leaf type and a symbol on it would collide in the interval index
func TestTypeExpressionSurvivesRangeQuery(t *testing.T) {
	pf := parseInline(t, "range.go", "package p\n\nfunc f(a Widget) {}\n")
	if pf == nil {
		return
	}

	rng, err := pf.FindSnippetRange([]byte("Widget"))
	if !assert.NoError(t, err) {
		return
	}

	nodes := pf.FindNodesWithinRange(rng)
	if !assert.Len(t, nodes, 1) {
		return
	}

	texpr, ok := nodes[0].(*ast.ASTTypeExpression)
	if assert.True(t, ok, "expected the type expression, got %T", nodes[0]) {
		assert.Equal(t, "Widget", texpr.Name)
	}
}

func TestTypeExpressionIsNotAGraphVertex(t *testing.T) {
	// A type reference is an edge to the declaration it resolves to, never a
	// vertex of its own
	pf := parseInline(t, "vertex.go", "package p\n\nfunc f(a Widget) {}\n")
	if pf == nil {
		return
	}

	fn := firstFunction(t, pf)
	assert.False(t, cog.IsNodeOfInterest(fn.Arguments[0].Type))
	assert.True(t, cog.IsNodeOfInterest(fn))
}

// A type used only in a signature must still pull its definition into the graph
func TestSearchSnippetResolvesSignatureTypes(t *testing.T) {
	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	wspace := cog.NewWorkspace(cwd)

	og, root, err := wspace.SearchSnippet(
		t.Context(),
		"./fixtures/golang/baseproj/method.go",
		"func (w *widget) Describe(prefix string) string {",
	)

	if !assert.NoError(t, err) {
		return
	}

	fn, ok := root.(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected root to be the enclosing function, got %T", root) {
		return
	}

	assert.Equal(t, "Describe", fn.Name.Name)

	adj, err := og.Graph.AdjacencyMap()
	if !assert.NoError(t, err) {
		return
	}

	pf := parseTestFile(t, "./fixtures/golang/baseproj/method.go")

	rng, err := pf.FindSnippetRange([]byte("widget struct{}"))
	if !assert.NoError(t, err) {
		return
	}

	widget := pf.FindTightestEnclosingNode(rng, cog.IsNodeOfInterest)
	if !assert.NotNil(t, widget, "expected the widget type to be a vertex candidate") {
		return
	}

	assert.Contains(t, adj, ast.GetHash(widget), "expected the receiver type in the graph")
}

func TestGolangClassifyImportType(t *testing.T) {
	out, err := exec.Command("go", "env", "GOROOT").Output()
	if !assert.NoError(t, err) {
		return
	}

	goroot := filepath.Clean(strings.TrimSpace(string(out)))

	// The classifier sees both workspace-relative and absolute paths
	workspace, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	relStdlib, err := filepath.Rel(workspace, filepath.Join(goroot, "src", "net", "http", "server.go"))
	if !assert.NoError(t, err) {
		return
	}

	cases := map[string]common.DependencyType{
		"../../../go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.3.darwin-arm64/src/fmt/print.go": common.StandardLibraryDependency,
		filepath.Join(goroot, "src", "fmt", "print.go"):                                          common.StandardLibraryDependency,
		relStdlib: common.StandardLibraryDependency,
		"../../../go/pkg/mod/github.com/spf13/pflag@v1.0.10/flag.go": common.PackageDependency,
		"vendor/github.com/tree-sitter/go-tree-sitter/node.go":       common.PackageDependency,
		"fixtures/golang/multidep/pkg/dep1/dep1.go":                  common.LocalDependency,
		filepath.Join(workspace, "cmd", "main.go"):                   common.LocalDependency,
	}

	for path, want := range cases {
		assert.Equal(t, want, languages.Golang.ClassifyImportType(common.NewSource(workspace, path, nil)), path)
	}
}
