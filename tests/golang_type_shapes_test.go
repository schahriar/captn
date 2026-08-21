package tests_test

import (
	"os"
	"testing"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/stretchr/testify/assert"
)

func namedFunction(t *testing.T, pf *cog.COGFile, name string) *ast.ASTFuncExpression {
	t.Helper()

	for _, chld := range pf.Module.Block.Children() {
		if fn, ok := chld.(*ast.ASTFuncExpression); ok && fn.Name != nil && fn.Name.Name == name {
			return fn
		}
	}

	assert.Failf(t, "function not found", "expected a function named %q in the module block", name)
	return nil
}

func typeNames(texprs []*ast.ASTTypeExpression) []string {
	names := []string{}
	for _, texpr := range texprs {
		names = append(names, texpr.Name)
	}
	return names
}

func eachNode(node ast.ASTNode, visit func(ast.ASTNode)) {
	visit(node)
	for _, child := range node.Children() {
		eachNode(child, visit)
	}
}

// A composite with no name of its own folds flat: the first named type is the
// head and the rest are its arguments, one level deep. Only a generic head owns
// its arguments.
func TestGolangTypeExpressionFoldsFlat(t *testing.T) {
	pf := parseInline(t, "fold.go", "package p\n\nfunc f(a Result[Widget, error], m map[string]map[int]Widget, h func(a Widget) Report) {}\n")
	if pf == nil {
		return
	}

	fn := firstFunction(t, pf)
	if !assert.Len(t, fn.Arguments, 3) {
		return
	}

	if assertTypeSource(t, fn.Arguments[0].Type, "Result"); fn.Arguments[0].Type != nil {
		assert.Equal(t, []string{"Widget", "error"}, typeNames(fn.Arguments[0].Type.Arguments))
	}

	if assertTypeSource(t, fn.Arguments[1].Type, "string"); fn.Arguments[1].Type != nil {
		assert.Equal(t, []string{"int", "Widget"}, typeNames(fn.Arguments[1].Type.Arguments))
		for _, arg := range fn.Arguments[1].Type.Arguments {
			assert.Empty(t, arg.Arguments, "flat fold never chains siblings")
		}
	}

	if assertTypeSource(t, fn.Arguments[2].Type, "Widget"); fn.Arguments[2].Type != nil {
		assert.Equal(t, []string{"Report"}, typeNames(fn.Arguments[2].Type.Arguments))
	}
}

// tree-sitter recovers `map[string]` and `[]` with a zero-width MISSING type
// identifier; that must never become a node
func TestGolangTypeExpressionSkipsMissingTypes(t *testing.T) {
	pf := parseInline(t, "missing.go", "package p\n\nfunc f(a map[string]) {}\nfunc g(a []) {}\nfunc h(a Result[]) {}\n")
	if pf == nil {
		return
	}

	eachNode(pf.Module, func(node ast.ASTNode) {
		if texpr, ok := node.(*ast.ASTTypeExpression); ok {
			assert.NotEmpty(t, texpr.Name, "zero-width type expression at %v", texpr.GetPosition())
		}
	})

	if f := namedFunction(t, pf, "f"); f != nil && assert.Len(t, f.Arguments, 1) {
		if assertTypeSource(t, f.Arguments[0].Type, "string"); f.Arguments[0].Type != nil {
			assert.Empty(t, f.Arguments[0].Type.Arguments)
		}
	}

	if g := namedFunction(t, pf, "g"); g != nil && assert.Len(t, g.Arguments, 1) {
		assert.Nil(t, g.Arguments[0].Type)
	}

	if h := namedFunction(t, pf, "h"); h != nil && assert.Len(t, h.Arguments, 1) {
		if assertTypeSource(t, h.Arguments[0].Type, "Result"); h.Arguments[0].Type != nil {
			assert.Empty(t, h.Arguments[0].Type.Arguments)
		}
	}
}

// Type parameters are arguments of the declaring function, ahead of the value
// parameters. Several names can share one declaration; each further name gets
// its own argument so a definition resolving onto it finds exactly one node.
func TestGolangTypeParametersAreArguments(t *testing.T) {
	pf := parseInline(t, "typeparams.go", "package p\n\nfunc Map[T, U any](s []T, f func(T) U) []U { return nil }\n")
	if pf == nil {
		return
	}

	fn := firstFunction(t, pf)
	if !assert.Len(t, fn.Arguments, 4) {
		return
	}

	if assert.NotNil(t, fn.Arguments[0].Identifier) {
		assert.Equal(t, "T", fn.Arguments[0].Identifier.Name)
		assertTypeSource(t, fn.Arguments[0].Type, "any")
	}

	if assert.NotNil(t, fn.Arguments[1].Identifier) {
		assert.Equal(t, "U", fn.Arguments[1].Identifier.Name)
		assert.Nil(t, fn.Arguments[1].Type)
		assert.Len(t, pf.FindNodesWithinRange(fn.Arguments[1].Identifier.GetPosition()), 1)
	}

	if assert.NotNil(t, fn.Arguments[2].Identifier) {
		assert.Equal(t, "s", fn.Arguments[2].Identifier.Name)
	}

	if assert.NotNil(t, fn.Arguments[3].Identifier) {
		assert.Equal(t, "f", fn.Arguments[3].Identifier.Name)
	}

	assertTypeSource(t, fn.ReturnType, "U")

	eachNode(pf.Module, func(node ast.ASTNode) {
		_, ok := node.(*ast.ASTDeclaration)
		assert.False(t, ok, "type parameters must not be declarations")
	})
}

func TestGolangSharedParameterType(t *testing.T) {
	pf := parseInline(t, "shared.go", "package p\n\nfunc f(a, b int) {}\n")
	if pf == nil {
		return
	}

	fn := firstFunction(t, pf)
	if !assert.Len(t, fn.Arguments, 2) {
		return
	}

	assert.Equal(t, "a, b int", fn.Arguments[0].GetStringSource())
	if assert.NotNil(t, fn.Arguments[0].Identifier) {
		assert.Equal(t, "a", fn.Arguments[0].Identifier.Name)
	}
	assertTypeSource(t, fn.Arguments[0].Type, "int")

	assert.Equal(t, "b", fn.Arguments[1].GetStringSource())
	if assert.NotNil(t, fn.Arguments[1].Identifier) {
		assert.Equal(t, "b", fn.Arguments[1].Identifier.Name)
	}
	assert.Nil(t, fn.Arguments[1].Type)
}

// A parameter list under a function type belongs to the type, not to the
// function being walked
func TestGolangFunctionTypeResultAddsNoArguments(t *testing.T) {
	pf := parseInline(t, "fnresult.go", "package p\n\nfunc f() func(a Widget) { return nil }\n")
	if pf == nil {
		return
	}

	fn := firstFunction(t, pf)
	assert.Empty(t, fn.Arguments)
	assertTypeSource(t, fn.ReturnType, "Widget")

	rng, err := pf.FindSnippetRange([]byte("Widget"))
	if !assert.NoError(t, err) {
		return
	}

	assert.Len(t, pf.FindNodesWithinRange(rng), 1)
}

// A multi-value result list lands in Arguments; that is the reference
// behaviour and stays as it is
func TestGolangMultiValueResultIsArguments(t *testing.T) {
	pf := parseInline(t, "multi.go", "package p\n\nfunc f() (Widget, error) { return nil, nil }\n")
	if pf == nil {
		return
	}

	fn := firstFunction(t, pf)
	if !assert.Len(t, fn.Arguments, 2) {
		return
	}

	assertTypeSource(t, fn.Arguments[0].Type, "Widget")
	assertTypeSource(t, fn.Arguments[1].Type, "error")
	assert.Nil(t, fn.ReturnType)
}

func TestGolangStructTypeFields(t *testing.T) {
	pf := parseInline(t, "struct.go", "package p\n\ntype S struct {\n\tName string\n\tpkg.Embedded\n\tf func(x int)\n\ta, b int\n}\n")
	if pf == nil {
		return
	}

	s := namedFunction(t, pf, "S")
	if s == nil {
		return
	}

	assert.Empty(t, s.Arguments, "field types must not add arguments")
	assert.Equal(t, "type S struct {\n\tName string\n\tpkg.Embedded\n\tf func(x int)\n\ta, b int\n}", s.GetStringSource())

	fields := s.Block.Children()
	if !assert.Len(t, fields, 4) {
		return
	}

	name, ok := fields[0].(*ast.ASTDeclaration)
	if assert.True(t, ok, "expected a Declaration, got %T", fields[0]) {
		if assert.Len(t, name.Names, 1) {
			assert.Equal(t, "Name", name.Names[0].Name)
		}
		assertTypeSource(t, name.Type, "string")
	}

	embedded, ok := fields[1].(*ast.ASTDeclaration)
	if assert.True(t, ok, "expected a Declaration, got %T", fields[1]) {
		assert.Empty(t, embedded.Names)
		if assertTypeSource(t, embedded.Type, "Embedded"); embedded.Type != nil && assert.NotNil(t, embedded.Type.Namespace) {
			assert.Equal(t, "pkg", embedded.Type.Namespace.Name)
		}
	}

	fnField, ok := fields[2].(*ast.ASTDeclaration)
	if assert.True(t, ok, "expected a Declaration, got %T", fields[2]) {
		assertTypeSource(t, fnField.Type, "int")
	}

	shared, ok := fields[3].(*ast.ASTDeclaration)
	if assert.True(t, ok, "expected a Declaration, got %T", fields[3]) {
		if assert.Len(t, shared.Names, 2) {
			assert.Equal(t, "a", shared.Names[0].Name)
			assert.Equal(t, "b", shared.Names[1].Name)
		}
		assertTypeSource(t, shared.Type, "int")
	}
}

func TestGolangInterfaceTypeMethods(t *testing.T) {
	pf := parseInline(t, "iface.go", "package p\n\ntype I interface { M(a Widget) Report; N() (Widget, error); io.Reader }\n")
	if pf == nil {
		return
	}

	iface := namedFunction(t, pf, "I")
	if iface == nil {
		return
	}

	assert.Empty(t, iface.Arguments, "method parameters must not leak into the interface")

	members := iface.Block.Children()
	if !assert.Len(t, members, 3) {
		return
	}

	m, ok := members[0].(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected a FuncExpression, got %T", members[0]) {
		if assert.NotNil(t, m.Name) {
			assert.Equal(t, "M", m.Name.Name)
		}
		if assert.Len(t, m.Arguments, 1) {
			assert.Equal(t, "a Widget", m.Arguments[0].GetStringSource())
			assertTypeSource(t, m.Arguments[0].Type, "Widget")
		}
		assertTypeSource(t, m.ReturnType, "Report")
		assert.True(t, cog.IsNodeOfInterest(m))
	}

	n, ok := members[1].(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected a FuncExpression, got %T", members[1]) {
		if assert.NotNil(t, n.Name) {
			assert.Equal(t, "N", n.Name.Name)
		}
		if assert.Len(t, n.Arguments, 2) {
			assertTypeSource(t, n.Arguments[0].Type, "Widget")
			assertTypeSource(t, n.Arguments[1].Type, "error")
		}
		assert.Nil(t, n.ReturnType)
	}

	embedded, ok := members[2].(*ast.ASTTypeExpression)
	if assert.True(t, ok, "expected a TypeExpression, got %T", members[2]) {
		assertTypeSource(t, embedded, "Reader")
	}

	// A definition resolves onto the method name, which must be the one node there
	nameRange, err := pf.FindSnippetRange([]byte("M"))
	if !assert.NoError(t, err) {
		return
	}

	nodes := pf.FindNodesWithinRange(nameRange)
	if assert.Len(t, nodes, 1) {
		assert.Same(t, m, nodes[0].NearestOrSelf(cog.IsNodeOfInterest))
	}
}

func TestGolangAliasAndFuncTypeDeclarations(t *testing.T) {
	pf := parseInline(t, "alias.go", "package p\n\ntype Alias = Widget\n\ntype Handler func(a Widget) Report\n")
	if pf == nil {
		return
	}

	alias := namedFunction(t, pf, "Alias")
	if alias != nil {
		assert.Empty(t, alias.Arguments)
		if assert.Len(t, alias.Block.Children(), 1) {
			texpr, ok := alias.Block.Children()[0].(*ast.ASTTypeExpression)
			if assert.True(t, ok, "expected a TypeExpression, got %T", alias.Block.Children()[0]) {
				assertTypeSource(t, texpr, "Widget")
			}
		}
	}

	handler := namedFunction(t, pf, "Handler")
	if handler != nil {
		assert.Empty(t, handler.Arguments, "a func type's parameters belong to the type")
		if assert.Len(t, handler.Block.Children(), 1) {
			texpr, ok := handler.Block.Children()[0].(*ast.ASTTypeExpression)
			if assert.True(t, ok, "expected a TypeExpression, got %T", handler.Block.Children()[0]) {
				assertTypeSource(t, texpr, "Widget")
				assert.Equal(t, []string{"Report"}, typeNames(texpr.Arguments))
			}
		}
	}
}

func TestGolangGroupedTypeDeclaration(t *testing.T) {
	pf := parseInline(t, "grouped.go", "package p\n\ntype (\n\tA int\n\tB string\n)\n")
	if pf == nil {
		return
	}

	a := namedFunction(t, pf, "A")
	if a != nil {
		assert.Equal(t, "A int", a.GetStringSource())
	}

	b := namedFunction(t, pf, "B")
	if b != nil {
		assert.Equal(t, "B string", b.GetStringSource())
	}
}

// A snippet starting at the `type` keyword must root at the type, not the module
func TestGolangTypeDeclarationRootsSnippet(t *testing.T) {
	pf := parseInline(t, "root.go", "package p\n\ntype Config struct {\n\tName string\n}\n")
	if pf == nil {
		return
	}

	rng, err := pf.FindSnippetRange([]byte("type Config struct {"))
	if !assert.NoError(t, err) {
		return
	}

	root := pf.FindTightestEnclosingNode(rng, cog.IsNodeOfInterest)
	fn, ok := root.(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected the type's FuncExpression, got %T", root) && assert.NotNil(t, fn.Name) {
		assert.Equal(t, "Config", fn.Name.Name)
	}
}

// A call with no identifiable callee is not emitted; its children still index
// under the enclosing block
func TestGolangVarSpecDeclaresEveryName(t *testing.T) {
	// A call to the second name of `var a, b func()` resolves onto `b`, which
	// needs a node of its own
	pf := parseInline(t, "vars.go", "package p\n\nvar hookA, hookB func() int\n")
	if pf == nil {
		return
	}

	decl, ok := pf.Module.Block.Children()[0].(*ast.ASTDeclaration)
	if !assert.True(t, ok, "expected a Declaration, got %T", pf.Module.Block.Children()[0]) {
		return
	}

	if assert.Len(t, decl.Names, 2) {
		assert.Equal(t, "hookA", decl.Names[0].Name)
		assert.Equal(t, "hookB", decl.Names[1].Name)
	}

	assertTypeSource(t, decl.Type, "int")
}

func TestGolangCallWithoutCalleeIsNotEmitted(t *testing.T) {
	pf := parseInline(t, "nocallee.go", "package p\n\nfunc z() {\n\tdefer func() { g() }()\n\tfns[i]()\n\t(f)()\n}\n")
	if pf == nil {
		return
	}

	calls := 0
	literals := 0

	eachNode(pf.Module, func(node ast.ASTNode) {
		switch n := node.(type) {
		case *ast.ASTCallExpression:
			calls++
			assert.NotNil(t, n.Symbol, "a call must carry a callee symbol")
		case *ast.ASTFuncExpression:
			if n.Name == nil {
				literals++
			}
		}
	})

	assert.Equal(t, 1, calls, "only g() has an identifiable callee")
	assert.Equal(t, 1, literals, "the deferred func literal still indexes under z")
}

func TestSearchSnippetSurvivesCallWithoutCallee(t *testing.T) {
	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	wspace := cog.NewWorkspace(cwd)

	_, root, err := wspace.SearchSnippet(t.Context(), "./fixtures/golang/baseproj/deferred.go", "defer func() {}()")
	if !assert.NoError(t, err) {
		return
	}

	fn, ok := root.(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected root to be the enclosing function, got %T", root) && assert.NotNil(t, fn.Name) {
		assert.Equal(t, "cleanup", fn.Name.Name)
	}
}

// A call through an interface resolves onto the method requirement, which is
// its own vertex under the interface; naming the interface in the snippet
// resolves the interface itself
func TestSearchSnippetResolvesInterfaceMethod(t *testing.T) {
	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	wspace := cog.NewWorkspace(cwd)
	pf := parseTestFile(t, "./fixtures/golang/baseproj/iface.go")

	store := namedFunction(t, pf, "Store")
	if store == nil || !assert.Len(t, store.Block.Children(), 1) {
		return
	}

	get, ok := store.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected the method requirement, got %T", store.Block.Children()[0]) {
		return
	}

	og, _, err := wspace.SearchSnippet(t.Context(), "./fixtures/golang/baseproj/iface.go", `s.Get("x")`)
	if !assert.NoError(t, err) {
		return
	}

	adj, err := og.Graph.AdjacencyMap()
	if !assert.NoError(t, err) {
		return
	}

	assert.Contains(t, adj, ast.GetHash(get), "expected the interface method in the graph")

	og, _, err = wspace.SearchSnippet(t.Context(), "./fixtures/golang/baseproj/iface.go", "func use(s Store) string {")
	if !assert.NoError(t, err) {
		return
	}

	adj, err = og.Graph.AdjacencyMap()
	if !assert.NoError(t, err) {
		return
	}

	assert.Contains(t, adj, ast.GetHash(store), "expected the interface type in the graph")
}

// Every use of a type parameter resolves back onto its declaration in the
// signature, which must hold exactly one node or the search hard-errors
func TestSearchSnippetResolvesTypeParameters(t *testing.T) {
	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	wspace := cog.NewWorkspace(cwd)

	_, root, err := wspace.SearchSnippet(t.Context(), "./fixtures/golang/baseproj/generic.go", "func Map[T, U any](s []T, f func(T) U) []U {")
	if !assert.NoError(t, err) {
		return
	}

	fn, ok := root.(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected root to be the generic function, got %T", root) && assert.NotNil(t, fn.Name) {
		assert.Equal(t, "Map", fn.Name.Name)
	}
}
