package tests_test

import (
	"testing"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/languages"
	"github.com/stretchr/testify/assert"
)

func TestTypeExpressionRust(t *testing.T) {
	pf := parseInline(t, "types.rs", "fn f(a: Widget, b: Vec<Widget>, c: HashMap<String, Widget>, d: pkg::Thing) -> Report {\n    a.report\n}\n")
	if pf == nil {
		return
	}

	fn := firstFunction(t, pf)
	if !assert.Len(t, fn.Arguments, 4) {
		return
	}

	assertTypeSource(t, fn.Arguments[0].Type, "Widget")

	if assertTypeSource(t, fn.Arguments[1].Type, "Vec"); fn.Arguments[1].Type != nil {
		if assert.Len(t, fn.Arguments[1].Type.Arguments, 1) {
			assertTypeSource(t, fn.Arguments[1].Type.Arguments[0], "Widget")
		}
	}

	if assertTypeSource(t, fn.Arguments[2].Type, "HashMap"); fn.Arguments[2].Type != nil {
		assert.Equal(t, []string{"String", "Widget"}, typeNames(fn.Arguments[2].Type.Arguments))
	}

	if assertTypeSource(t, fn.Arguments[3].Type, "Thing"); fn.Arguments[3].Type != nil {
		if assert.NotNil(t, fn.Arguments[3].Type.Namespace) {
			assert.Equal(t, "pkg", fn.Arguments[3].Type.Namespace.Name)
		}
	}

	assertTypeSource(t, fn.ReturnType, "Report")
}

// A composite with no name of its own folds flat: the first named type is the
// head and the rest are its arguments
func TestRustTypeExpressionFoldsFlat(t *testing.T) {
	pf := parseInline(t, "fold.rs", "fn f(m: &[Widget], p: (Widget, Report), h: fn(Widget) -> Report) {}\n")
	if pf == nil {
		return
	}

	fn := firstFunction(t, pf)
	if !assert.Len(t, fn.Arguments, 3) {
		return
	}

	if assertTypeSource(t, fn.Arguments[0].Type, "Widget"); fn.Arguments[0].Type != nil {
		assert.Empty(t, fn.Arguments[0].Type.Arguments)
	}

	if assertTypeSource(t, fn.Arguments[1].Type, "Widget"); fn.Arguments[1].Type != nil {
		assert.Equal(t, []string{"Report"}, typeNames(fn.Arguments[1].Type.Arguments))
		for _, arg := range fn.Arguments[1].Type.Arguments {
			assert.Empty(t, arg.Arguments, "flat fold never chains siblings")
		}
	}

	if assertTypeSource(t, fn.Arguments[2].Type, "Widget"); fn.Arguments[2].Type != nil {
		assert.Equal(t, []string{"Report"}, typeNames(fn.Arguments[2].Type.Arguments))
	}
}

// Primitive types have no definition site; they must never become type
// expressions or the search would chase definitions that cannot answer
func TestRustPrimitiveTypesAreSkipped(t *testing.T) {
	pf := parseInline(t, "prim.rs", "fn f(a: i32, s: &str, ok: bool) -> usize {\n    0\n}\n")
	if pf == nil {
		return
	}

	fn := firstFunction(t, pf)
	if !assert.Len(t, fn.Arguments, 3) {
		return
	}

	for _, arg := range fn.Arguments {
		assert.Nil(t, arg.Type)
	}

	assert.Nil(t, fn.ReturnType)
}

// Type parameters are arguments of the declaring function, ahead of the value
// parameters, so a definition resolving onto them finds exactly one node
func TestRustTypeParametersAreArguments(t *testing.T) {
	pf := parseInline(t, "typeparams.rs", "fn map<T, U: Clone>(s: Vec<T>, f: fn(T) -> U) -> Vec<U> {\n    Vec::new()\n}\n")
	if pf == nil {
		return
	}

	fn := firstFunction(t, pf)
	if !assert.Len(t, fn.Arguments, 4) {
		return
	}

	if assert.NotNil(t, fn.Arguments[0].Identifier) {
		assert.Equal(t, "T", fn.Arguments[0].Identifier.Name)
		assert.Nil(t, fn.Arguments[0].Type)
	}

	if assert.NotNil(t, fn.Arguments[1].Identifier) {
		assert.Equal(t, "U", fn.Arguments[1].Identifier.Name)
		assertTypeSource(t, fn.Arguments[1].Type, "Clone")
		assert.Len(t, pf.FindNodesWithinRange(fn.Arguments[1].Identifier.GetPosition()), 1)
	}

	if assert.NotNil(t, fn.Arguments[2].Identifier) {
		assert.Equal(t, "s", fn.Arguments[2].Identifier.Name)
	}

	if assert.NotNil(t, fn.Arguments[3].Identifier) {
		assert.Equal(t, "f", fn.Arguments[3].Identifier.Name)
	}

	if assertTypeSource(t, fn.ReturnType, "Vec"); fn.ReturnType != nil {
		assert.Equal(t, []string{"U"}, typeNames(fn.ReturnType.Arguments))
	}

	eachNode(pf.Module, func(node ast.ASTNode) {
		_, ok := node.(*ast.ASTDeclaration)
		assert.False(t, ok, "type parameters must not be declarations")
	})
}

func TestRustStructFields(t *testing.T) {
	pf := parseInline(t, "struct.rs", "struct S {\n    name: String,\n    parts: Vec<Widget>,\n}\n\nstruct P(Widget, Report);\n\nstruct Marker;\n")
	if pf == nil {
		return
	}

	s := namedFunction(t, pf, "S")
	if s == nil {
		return
	}

	assert.Empty(t, s.Arguments, "field types must not add arguments")

	fields := s.Block.Children()
	if !assert.Len(t, fields, 2) {
		return
	}

	name, ok := fields[0].(*ast.ASTDeclaration)
	if assert.True(t, ok, "expected a Declaration, got %T", fields[0]) {
		if assert.Len(t, name.Names, 1) {
			assert.Equal(t, "name", name.Names[0].Name)
		}
		assertTypeSource(t, name.Type, "String")
	}

	parts, ok := fields[1].(*ast.ASTDeclaration)
	if assert.True(t, ok, "expected a Declaration, got %T", fields[1]) {
		assertTypeSource(t, parts.Type, "Vec")
	}

	// A tuple struct has no field names; its types fold onto one declaration
	p := namedFunction(t, pf, "P")
	if p != nil && assert.Len(t, p.Block.Children(), 1) {
		decl, ok := p.Block.Children()[0].(*ast.ASTDeclaration)
		if assert.True(t, ok, "expected a Declaration, got %T", p.Block.Children()[0]) {
			assert.Empty(t, decl.Names)
			if assertTypeSource(t, decl.Type, "Widget"); decl.Type != nil {
				assert.Equal(t, []string{"Report"}, typeNames(decl.Type.Arguments))
			}
		}
	}

	marker := namedFunction(t, pf, "Marker")
	if marker != nil {
		assert.Empty(t, marker.Block.Children())
	}
}

func TestRustEnumVariants(t *testing.T) {
	pf := parseInline(t, "enum.rs", "enum Shape {\n    Circle(Radius),\n    Rect { w: Width, h: Width },\n    Empty,\n}\n")
	if pf == nil {
		return
	}

	shape := namedFunction(t, pf, "Shape")
	if shape == nil {
		return
	}

	variants := shape.Block.Children()
	if !assert.Len(t, variants, 3) {
		return
	}

	names := declaredNames(shape)
	assert.Contains(t, names, "Circle")
	assert.Contains(t, names, "Rect")
	assert.Contains(t, names, "Empty")

	circle, ok := variants[0].(*ast.ASTDeclaration)
	if assert.True(t, ok, "expected a Declaration, got %T", variants[0]) {
		assertTypeSource(t, circle.Type, "Radius")
	}
}

func TestRustAliasDeclaresNames(t *testing.T) {
	// A resolved type reference needs a node at its definition; without this
	// SearchSnippet fails with "expected 1 node for dependent node"
	pf := parseInline(t, "alias.rs", "type Alias = Widget;\n\ntype Pair<T> = (T, T);\n")
	if pf == nil {
		return
	}

	names := declaredNames(pf.Module)
	assert.Contains(t, names, "Alias")
	assert.Contains(t, names, "Pair")
	assert.Contains(t, names, "T")

	decl, ok := pf.Module.Block.Children()[0].(*ast.ASTDeclaration)
	if assert.True(t, ok, "expected a Declaration for the alias, got %T", pf.Module.Block.Children()[0]) {
		assertTypeSource(t, decl.Type, "Widget")
	}
}

func TestRustTraitAndImplShapes(t *testing.T) {
	pf := parseInline(t, "impl.rs", "trait Describe: Base {\n    type Output;\n    fn describe(&self) -> Report;\n}\n\nimpl Describe for Widget {\n    type Output = Report;\n    fn describe(&self) -> Report {\n        make()\n    }\n}\n")
	if pf == nil {
		return
	}

	describe := namedFunction(t, pf, "Describe")
	if describe != nil {
		// Supertrait, associated type, and the requirement hang off the trait
		members := describe.Block.Children()

		if assert.Len(t, members, 3) {
			supertrait, ok := members[0].(*ast.ASTTypeExpression)
			if assert.True(t, ok, "expected the supertrait, got %T", members[0]) {
				assertTypeSource(t, supertrait, "Base")
			}

			output, ok := members[1].(*ast.ASTDeclaration)
			if assert.True(t, ok, "expected the associated type, got %T", members[1]) {
				if assert.Len(t, output.Names, 1) {
					assert.Equal(t, "Output", output.Names[0].Name)
				}
			}

			_, ok = members[2].(*ast.ASTFuncExpression)
			assert.True(t, ok, "expected the requirement, got %T", members[2])
		}
	}

	impl := namedFunction(t, pf, "Widget")
	if impl == nil {
		return
	}

	members := impl.Block.Children()
	if !assert.Len(t, members, 3) {
		return
	}

	trait, ok := members[0].(*ast.ASTTypeExpression)
	if assert.True(t, ok, "expected the implemented trait, got %T", members[0]) {
		assertTypeSource(t, trait, "Describe")
	}

	output, ok := members[1].(*ast.ASTDeclaration)
	if assert.True(t, ok, "expected the type assignment, got %T", members[1]) {
		if assert.Len(t, output.Names, 1) {
			assert.Equal(t, "Output", output.Names[0].Name)
		}
		assertTypeSource(t, output.Type, "Report")
	}
}

func TestRustGenericImplNamesSelfType(t *testing.T) {
	pf := parseInline(t, "genimpl.rs", "impl<T: Clone> Stack<T> {\n    fn push(&mut self, item: T) {}\n}\n")
	if pf == nil {
		return
	}

	impl := firstFunction(t, pf)

	if assert.NotNil(t, impl.Name) {
		assert.Equal(t, "Stack", impl.Name.Name)
	}

	if assert.Len(t, impl.Arguments, 1) && assert.NotNil(t, impl.Arguments[0].Identifier) {
		assert.Equal(t, "T", impl.Arguments[0].Identifier.Name)
		assertTypeSource(t, impl.Arguments[0].Type, "Clone")
	}
}

// Construction is the call-shaped use of a type; the name must resolve like a
// call so the type's definition joins the graph
func TestRustStructExpression(t *testing.T) {
	pf := parseInline(t, "construct.rs", "fn build() -> Widget {\n    Widget { label: make() }\n}\n")
	if pf == nil {
		return
	}

	fn := firstFunction(t, pf)

	texpr, ok := fn.Block.Children()[0].(*ast.ASTTypeExpression)
	if assert.True(t, ok, "expected the constructed type, got %T", fn.Block.Children()[0]) {
		assertTypeSource(t, texpr, "Widget")
	}

	calls := 0
	eachNode(pf.Module, func(node ast.ASTNode) {
		if _, ok := node.(*ast.ASTCallExpression); ok {
			calls++
		}
	})
	assert.Equal(t, 1, calls, "the field initializer call still indexes")
}

// A macro call resolves like a function call; its token-tree arguments are
// not parsed and contribute no edges
func TestRustMacroInvocation(t *testing.T) {
	pf := parseInline(t, "macro.rs", "macro_rules! shout {\n    ($x:expr) => { $x };\n}\n\nfn f() {\n    shout!(g());\n    std::println!(\"x\");\n}\n")
	if pf == nil {
		return
	}

	names := declaredNames(pf.Module)
	assert.Contains(t, names, "shout", "a macro definition declares its name")

	f := namedFunction(t, pf, "f")
	if f == nil || !assert.Len(t, f.Block.Children(), 2) {
		return
	}

	shout, ok := f.Block.Children()[0].(*ast.ASTCallExpression)
	if assert.True(t, ok, "expected the macro call, got %T", f.Block.Children()[0]) {
		if assert.NotNil(t, shout.Symbol) {
			assert.Equal(t, "shout", shout.Symbol.Name)
		}
		assert.Empty(t, shout.Virtual, "token-tree arguments are not parsed")
	}

	println, ok := f.Block.Children()[1].(*ast.ASTCallExpression)
	if assert.True(t, ok, "expected the scoped macro call, got %T", f.Block.Children()[1]) {
		if assert.NotNil(t, println.Symbol) {
			assert.Equal(t, "println", println.Symbol.Name)
		}
		if assert.NotNil(t, println.Namespace) {
			assert.Equal(t, "std", println.Namespace.Name)
		}
	}
}

// A call resolving into a binding needs a node at the binding's identifier
func TestRustBindingsDeclareNames(t *testing.T) {
	pf := parseInline(t, "bindings.rs", "fn f(v: Option<Hook>) {\n    if let Some(h) = v {\n        h();\n    }\n    match v {\n        Some(g) => g(),\n        None => {}\n    }\n    for cb in hooks() {\n        cb();\n    }\n}\n")
	if pf == nil {
		return
	}

	names := declaredNames(pf.Module)
	assert.Contains(t, names, "h")
	assert.Contains(t, names, "g")
	assert.Contains(t, names, "cb")
}

func TestRustCallWithoutCalleeIsNotEmitted(t *testing.T) {
	pf := parseInline(t, "nocallee.rs", "fn z(fns: Vec<fn()>) {\n    (fns[0])();\n    fns[1]();\n    g();\n}\n")
	if pf == nil {
		return
	}

	calls := 0

	eachNode(pf.Module, func(node ast.ASTNode) {
		if call, ok := node.(*ast.ASTCallExpression); ok {
			calls++
			assert.NotNil(t, call.Symbol, "a call must carry a callee symbol")
		}
	})

	assert.Equal(t, 1, calls, "only g() has an identifiable callee")
}

func TestRustTypeExpressionIsNotAGraphVertex(t *testing.T) {
	pf := parseInline(t, "vertex.rs", "fn f(a: Widget) {}\n")
	if pf == nil {
		return
	}

	fn := firstFunction(t, pf)
	assert.False(t, cog.IsNodeOfInterest(fn.Arguments[0].Type))
	assert.True(t, cog.IsNodeOfInterest(fn))
}

// A leaf type and a symbol on it would collide in the interval index
func TestRustTypeExpressionSurvivesRangeQuery(t *testing.T) {
	pf := parseInline(t, "range.rs", "fn f(a: Widget) {}\n")
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

func TestRustClassifyImportType(t *testing.T) {
	// Paths mirror what rust-analyzer actually returns, after
	// filepath.ToSlash; both absolute and workspace-relative forms occur
	cases := map[string]common.DependencyType{
		"/Users/u/.rustup/toolchains/stable-aarch64-apple-darwin/lib/rustlib/src/rust/library/std/src/collections/hash/map.rs": common.StandardLibraryDependency,
		"../../.rustup/toolchains/stable-aarch64-apple-darwin/lib/rustlib/src/rust/library/alloc/src/string.rs":                common.StandardLibraryDependency,
		"C:/Users/u/.rustup/toolchains/stable-x86_64-pc-windows-msvc/lib/rustlib/src/rust/library/core/src/option.rs":          common.StandardLibraryDependency,
		"/Users/u/.cargo/registry/src/index.crates.io-6f17d22bba15001f/serde-1.0.219/src/lib.rs":                               common.PackageDependency,
		"/Users/u/.cargo/git/checkouts/tokio-abc123/def456/tokio/src/lib.rs":                                                   common.PackageDependency,
		"vendor/serde/src/lib.rs":                                                                                              common.PackageDependency,
		"fixtures/rust/multidep/src/dep1.rs":                                                                                   common.LocalDependency,
		"/Users/u/project/src/main.rs":                                                                                         common.LocalDependency,
	}

	for path, want := range cases {
		assert.Equal(t, want, languages.Rust.ClassifyImportType(common.NewSource("", path, nil)), path)
	}
}
