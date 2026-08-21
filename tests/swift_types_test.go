package tests_test

import (
	"os"
	"testing"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/stretchr/testify/assert"
)

// swiftFunctionNamed finds the FuncExpression declared with the given name anywhere
// under the node
func swiftFunctionNamed(node ast.ASTNode, name string) *ast.ASTFuncExpression {
	if fn, ok := node.(*ast.ASTFuncExpression); ok && fn.Name != nil && fn.Name.Name == name {
		return fn
	}

	for _, child := range node.Children() {
		if fn := swiftFunctionNamed(child, name); fn != nil {
			return fn
		}
	}

	return nil
}

// swiftDeclarationNamed finds the Declaration carrying the given name anywhere
// under the node
func swiftDeclarationNamed(node ast.ASTNode, name string) *ast.ASTDeclaration {
	if decl, ok := node.(*ast.ASTDeclaration); ok {
		for _, sym := range decl.Names {
			if sym.Name == name {
				return decl
			}
		}
	}

	for _, child := range node.Children() {
		if decl := swiftDeclarationNamed(child, name); decl != nil {
			return decl
		}
	}

	return nil
}

func assertTypeArguments(t *testing.T, texpr *ast.ASTTypeExpression, want ...string) {
	t.Helper()

	if !assert.NotNil(t, texpr) || !assert.Len(t, texpr.Arguments, len(want)) {
		return
	}

	for i, name := range want {
		assertTypeSource(t, texpr.Arguments[i], name)
	}
}

// A type parameter is a definition site: a resolved use of T lands on the
// parameter name, which needs exactly one node. It becomes an argument of the
// declaring function or type, ahead of the value parameters, never a
// declaration in the block.
func TestSwiftTypeParametersAreArguments(t *testing.T) {
	pf := parseSwiftInline(t, "func ident<T: Equatable, U>(v: T) -> T {\n    return v\n}\n\nclass Box<W> {\n    func get(v: W) -> W {\n        return v\n    }\n}\n\nprotocol P {\n    func req<Z>(z: Z)\n}\n")
	if pf == nil {
		return
	}

	ident := swiftFunctionNamed(pf.Module, "ident")
	if assert.NotNil(t, ident) && assert.Len(t, ident.Arguments, 3) {
		if assert.NotNil(t, ident.Arguments[0].Identifier) {
			assert.Equal(t, "T", ident.Arguments[0].Identifier.Name)
			assert.Equal(t, "T", ident.Arguments[0].Identifier.GetStringSource())
		}
		assertTypeSource(t, ident.Arguments[0].Type, "Equatable")

		if assert.NotNil(t, ident.Arguments[1].Identifier) {
			assert.Equal(t, "U", ident.Arguments[1].Identifier.Name)
		}
		assert.Nil(t, ident.Arguments[1].Type)

		if assert.NotNil(t, ident.Arguments[2].Identifier) {
			assert.Equal(t, "v", ident.Arguments[2].Identifier.Name)
		}
		assertTypeSource(t, ident.Arguments[2].Type, "T")
	}

	box := swiftFunctionNamed(pf.Module, "Box")
	if assert.NotNil(t, box) && assert.Len(t, box.Arguments, 1) && assert.NotNil(t, box.Arguments[0].Identifier) {
		assert.Equal(t, "W", box.Arguments[0].Identifier.Name)
	}

	req := swiftFunctionNamed(pf.Module, "req")
	if assert.NotNil(t, req) && assert.Len(t, req.Arguments, 2) && assert.NotNil(t, req.Arguments[0].Identifier) {
		assert.Equal(t, "Z", req.Arguments[0].Identifier.Name)
	}

	assert.Nil(t, swiftDeclarationNamed(pf.Module, "T"), "a type parameter must not become a declaration")
}

// A type parameter without a constraint shares its range with the argument
// wrapping it; the symbol wins the interval index, so a definition resolving
// onto the parameter still finds exactly one node
func TestSwiftTypeParameterSurvivesRangeQuery(t *testing.T) {
	pf := parseSwiftInline(t, "func ident<T>(v: T) -> T {\n    return v\n}\n")
	if pf == nil {
		return
	}

	rng, err := pf.FindSnippetRange([]byte("T"))
	if !assert.NoError(t, err) {
		return
	}

	nodes := pf.FindNodesWithinRange(rng)
	if !assert.Len(t, nodes, 1) {
		return
	}

	sym, ok := nodes[0].(*ast.ASTSymbol)
	if assert.True(t, ok, "expected the parameter symbol, got %T", nodes[0]) {
		assert.Equal(t, "T", sym.Name)
	}
}

// typealias and associatedtype declare a name with no members, so a
// Declaration is enough: the name for a definition to land on and the aliased
// type hanging off it
func TestSwiftTypeAliasDeclares(t *testing.T) {
	pf := parseSwiftInline(t, "typealias Label = String\n\nprotocol P {\n    associatedtype Item\n    typealias Pair = (Item, Label)\n}\n")
	if pf == nil {
		return
	}

	label := swiftDeclarationNamed(pf.Module, "Label")
	if assert.NotNil(t, label) && assert.Len(t, label.Names, 1) {
		assert.Equal(t, "Label", label.Names[0].GetStringSource())
		assertTypeSource(t, label.Type, "String")
	}

	item := swiftDeclarationNamed(pf.Module, "Item")
	if assert.NotNil(t, item) {
		assert.Nil(t, item.Type)
	}

	pair := swiftDeclarationNamed(pf.Module, "Pair")
	if assert.NotNil(t, pair) {
		if assertTypeSource(t, pair.Type, "Item"); pair.Type != nil {
			assertTypeArguments(t, pair.Type, "Label")
		}
	}

	proto := swiftFunctionNamed(pf.Module, "P")
	if assert.NotNil(t, proto) {
		assert.Same(t, proto.Block, item.GetParent(), "an associatedtype lives in the protocol block")
	}
}

// A generic alias declares its parameters too, so a T on the right-hand side
// resolves; an associated type's constraint and default are its type
func TestSwiftGenericAliasAndConstrainedAssociatedType(t *testing.T) {
	pf := parseSwiftInline(t, "typealias Handler<T> = (T) -> Void\n\nprotocol P {\n    associatedtype Item: Widget\n    associatedtype Def = Gadget\n    associatedtype Both: Hashable = String\n}\n")
	if pf == nil {
		return
	}

	handler := swiftDeclarationNamed(pf.Module, "Handler")
	if assert.NotNil(t, handler) && assert.Len(t, handler.Names, 2) {
		assert.Equal(t, "T", handler.Names[1].GetStringSource())
		if assertTypeSource(t, handler.Type, "T"); handler.Type != nil {
			assertTypeArguments(t, handler.Type, "Void")
		}
	}

	item := swiftDeclarationNamed(pf.Module, "Item")
	if assert.NotNil(t, item) {
		assertTypeSource(t, item.Type, "Widget")
	}

	def := swiftDeclarationNamed(pf.Module, "Def")
	if assert.NotNil(t, def) {
		assertTypeSource(t, def.Type, "Gadget")
	}

	both := swiftDeclarationNamed(pf.Module, "Both")
	if assert.NotNil(t, both) {
		if assertTypeSource(t, both.Type, "Hashable"); both.Type != nil {
			assertTypeArguments(t, both.Type, "String")
		}
	}
}

// Every named type in a use gets exactly one node on its own identifier. A
// dotted type keeps its last segment as head and the one before it as
// namespace; generic arguments from every segment are kept flat; a trailing
// .Type is the metatype marker and not a type
func TestSwiftDottedAndGenericTypes(t *testing.T) {
	pf := parseSwiftInline(t, "func f(a: Foo<A>.Bar<B>, b: Foo.Bar.Baz, c: Gadget.Type, d: Gadget.Protocol) {}\n")
	if pf == nil {
		return
	}

	fn := firstFunction(t, pf)
	if !assert.Len(t, fn.Arguments, 4) {
		return
	}

	if assertTypeSource(t, fn.Arguments[0].Type, "Bar"); fn.Arguments[0].Type != nil {
		if assert.NotNil(t, fn.Arguments[0].Type.Namespace) {
			assert.Equal(t, "Foo", fn.Arguments[0].Type.Namespace.Name)
		}
		assertTypeArguments(t, fn.Arguments[0].Type, "A", "B")
	}

	if assertTypeSource(t, fn.Arguments[1].Type, "Baz"); fn.Arguments[1].Type != nil {
		if assert.NotNil(t, fn.Arguments[1].Type.Namespace) {
			assert.Equal(t, "Bar", fn.Arguments[1].Type.Namespace.Name)
		}
		assert.Empty(t, fn.Arguments[1].Type.Arguments)
	}

	if assertTypeSource(t, fn.Arguments[2].Type, "Gadget"); fn.Arguments[2].Type != nil {
		assert.Nil(t, fn.Arguments[2].Type.Namespace)
	}

	assertTypeSource(t, fn.Arguments[3].Type, "Gadget")
}

// A composite with no name of its own folds flat: the first named type is
// the head and every other one hangs off it as an argument, one level deep
func TestSwiftHeadlessTypesFoldFlat(t *testing.T) {
	pf := parseSwiftInline(t, "func f(a: [String: Widget], b: Widget?, c: [String: [Int: Widget]], d: (Widget, Error), e: (Int) -> Widget, g: any P, h: P & Q, i: Array<[Widget]>) -> some View {}\n")
	if pf == nil {
		return
	}

	fn := firstFunction(t, pf)
	if !assert.Len(t, fn.Arguments, 8) {
		return
	}

	if assertTypeSource(t, fn.Arguments[0].Type, "String"); fn.Arguments[0].Type != nil {
		assertTypeArguments(t, fn.Arguments[0].Type, "Widget")
	}

	if assertTypeSource(t, fn.Arguments[1].Type, "Widget"); fn.Arguments[1].Type != nil {
		assert.Empty(t, fn.Arguments[1].Type.Arguments)
	}

	if assertTypeSource(t, fn.Arguments[2].Type, "String"); fn.Arguments[2].Type != nil {
		assertTypeArguments(t, fn.Arguments[2].Type, "Int", "Widget")
		assert.Empty(t, fn.Arguments[2].Type.Arguments[0].Arguments, "a nested composite must not chain")
	}

	if assertTypeSource(t, fn.Arguments[3].Type, "Widget"); fn.Arguments[3].Type != nil {
		assertTypeArguments(t, fn.Arguments[3].Type, "Error")
	}

	if assertTypeSource(t, fn.Arguments[4].Type, "Int"); fn.Arguments[4].Type != nil {
		assertTypeArguments(t, fn.Arguments[4].Type, "Widget")
	}

	assertTypeSource(t, fn.Arguments[5].Type, "P")

	if assertTypeSource(t, fn.Arguments[6].Type, "P"); fn.Arguments[6].Type != nil {
		assertTypeArguments(t, fn.Arguments[6].Type, "Q")
	}

	// A generic head owns its arguments even when they are composites
	if assertTypeSource(t, fn.Arguments[7].Type, "Array"); fn.Arguments[7].Type != nil {
		assertTypeArguments(t, fn.Arguments[7].Type, "Widget")
	}

	assertTypeSource(t, fn.ReturnType, "View")
}

// The grammar files `-> @MainActor Widget` with the attribute first under the
// return_type field; the attribute spells a name but is never a type
func TestSwiftAttributedTypes(t *testing.T) {
	pf := parseSwiftInline(t, "func f(a: Widget, b: @Sendable () -> Void) -> @MainActor Widget { return a }\n\nfunc s() -> @Sendable () -> Void {}\n\nlet h: @Sendable () -> Void = {}\n")
	if pf == nil {
		return
	}

	f := swiftFunctionNamed(pf.Module, "f")
	if assert.NotNil(t, f) {
		assertTypeSource(t, f.ReturnType, "Widget")

		if assert.Len(t, f.Arguments, 2) {
			assertTypeSource(t, f.Arguments[1].Type, "Void")
		}
	}

	s := swiftFunctionNamed(pf.Module, "s")
	if assert.NotNil(t, s) {
		assertTypeSource(t, s.ReturnType, "Void")
	}

	h := swiftDeclarationNamed(pf.Module, "h")
	if assert.NotNil(t, h) {
		assertTypeSource(t, h.Type, "Void")
	}

	names := []string{}
	for _, node := range pf.FindNodesWithinRange(pf.Module.GetPosition()) {
		if texpr, ok := node.(*ast.ASTTypeExpression); ok {
			names = append(names, texpr.Name)
		}
	}

	assert.NotContains(t, names, "MainActor")
	assert.NotContains(t, names, "Sendable")
}

// Return types keep their shape across the forms Swift writes them in
func TestSwiftReturnTypeForms(t *testing.T) {
	cases := map[string]string{
		"func f() -> Widget? {}\n":         "Widget",
		"func f() -> [Widget] {}\n":        "Widget",
		"func f() -> (Widget, Error) {}\n": "Widget",
		"func f() -> some P {}\n":          "P",
		"func f() -> any P {}\n":           "P",
		"func f() -> Widget! {}\n":         "Widget",
	}

	for source, want := range cases {
		pf := parseSwiftInline(t, source)
		if pf == nil {
			continue
		}

		fn := firstFunction(t, pf)
		if fn == nil {
			continue
		}

		if assert.NotNil(t, fn.ReturnType, source) {
			assert.Equal(t, want, fn.ReturnType.Name, source)
		}
	}
}

func TestSwiftPropertyDeclarationType(t *testing.T) {
	pf := parseSwiftInline(t, "let x: Widget = make()\n\nprotocol P {\n    var items: [Item] { get }\n}\n\nenum Kind {\n    case tagged(String)\n}\n")
	if pf == nil {
		return
	}

	x := swiftDeclarationNamed(pf.Module, "x")
	if assert.NotNil(t, x) {
		assertTypeSource(t, x.Type, "Widget")

		if assert.Len(t, x.Virtual, 1) {
			_, ok := x.Virtual[0].(*ast.ASTCallExpression)
			assert.True(t, ok, "the right-hand side call still lands in Virtual")
		}
	}

	items := swiftDeclarationNamed(pf.Module, "items")
	if assert.NotNil(t, items) {
		assertTypeSource(t, items.Type, "Item")
	}

	tagged := swiftDeclarationNamed(pf.Module, "tagged")
	if assert.NotNil(t, tagged) {
		assert.Nil(t, tagged.Type, "an enum case carries no annotation")
	}
}

// A generated interface is nothing but bodyless declarations, so the grammar
// makes the whole file one ERROR node and a swallowed type leaves its name as
// a bare identifier after the declaring keyword. That name is what a
// definition resolves onto. The source is the head of the Bool interface
// sourcekit-lsp generates, cut mid-body the way a file being typed is.
func TestSwiftErrorRecoveryKeepsTypeNames(t *testing.T) {
	pf := parseSwiftInline(t, "@frozen public struct Bool : Sendable {\n    public init()\n    @inlinable public init(_ value: Bool)\n    @inlinable public static func random<T>(using generator: inout T) -> Bool where T : RandomNumberGenerator\n")
	if pf == nil {
		return
	}

	names := declaredNames(pf.Module)
	assert.Contains(t, names, "Bool")
	assert.Contains(t, names, "random")

	// Modifiers and references also surface as bare identifiers inside an
	// ERROR; only the one after a declaring keyword is a declared name
	assert.NotContains(t, names, "public")
	assert.NotContains(t, names, "Sendable")
	assert.NotContains(t, names, "RandomNumberGenerator")

	// A definition resolving onto the recovered name must find exactly one node
	rng, err := pf.FindSnippetRange([]byte("Bool"))
	if !assert.NoError(t, err) {
		return
	}

	nodes := pf.FindNodesWithinRange(rng)
	if assert.Len(t, nodes, 1) {
		sym, ok := nodes[0].(*ast.ASTSymbol)
		if assert.True(t, ok, "expected a symbol on the recovered name, got %T", nodes[0]) {
			assert.Equal(t, "Bool", sym.Name)
		}
	}
}

// A constructor call resolves onto the init keyword. Recovery wraps a
// swallowed init in an ERROR node of its own, which stands in for the name.
func TestSwiftErrorRecoveryKeepsInit(t *testing.T) {
	pf := parseSwiftInline(t, "public func uppercased() -> String\n\n@inlinable public init<T>(_ value: T) where T : LosslessStringConvertible\n")
	if pf == nil {
		return
	}

	rng, err := pf.FindSnippetRange([]byte("init"))
	if !assert.NoError(t, err) {
		return
	}

	nodes := pf.FindNodesWithinRange(rng)
	if assert.Len(t, nodes, 1) {
		sym, ok := nodes[0].(*ast.ASTSymbol)
		if assert.True(t, ok, "expected a symbol on init, got %T", nodes[0]) {
			assert.Equal(t, "init", sym.Name)
		}
	}
}

// Every type used in a signature must resolve to exactly one node, or the
// search hard-errors. Standard library types resolve into generated
// interfaces, where the name survives only through error recovery.
func TestSwiftSearchSnippetResolvesSignatureTypes(t *testing.T) {
	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	wspace := cog.NewWorkspace(cwd)

	cases := []struct {
		file    string
		snippet string
		root    string
	}{
		{"./fixtures/swift/baseproj/Sources/BaseProj/simple.swift", "func x(v: Int) -> String {", "x"},
		{"./fixtures/swift/baseproj/Sources/BaseProj/simple.swift", "String(v * 2)", "x"},
		{"./fixtures/swift/baseproj/Sources/BaseProj/method.swift", "func describe(prefix: String) -> String {", "describe"},
		{"./fixtures/swift/baseproj/Sources/BaseProj/types.swift", "func ident<T>(v: T) -> T {", "ident"},
		{"./fixtures/swift/baseproj/Sources/BaseProj/types.swift", "func label(v: Label) -> Label {", "label"},
		{"./fixtures/swift/baseproj/Sources/BaseProj/types.swift", "func get(v: U) -> U {", "get"},
		{"./fixtures/swift/baseproj/Sources/BaseProj/types.swift", "func v() -> Void {", "v"},
		{"./fixtures/swift/baseproj/Sources/BaseProj/types.swift", "func b(x: Bool, y: Double) {", "b"},
		// Recovered names that are not plain identifiers: `struct Range` leaves an
		// ERROR node, `func prefix` the keyword token, `protocol Sequence` a fragment
		{"./fixtures/swift/baseproj/Sources/BaseProj/types.swift", "func ranges(r: Range<Int>, c: ClosedRange<Int>, xs: [Int]) -> Int {", "ranges"},
		{"./fixtures/swift/baseproj/Sources/BaseProj/types.swift", "let p = xs.prefix(2)", "ranges"},
		{"./fixtures/swift/baseproj/Sources/BaseProj/types.swift", "for i in stride(from: 0, to: 10, by: 2) {", "ranges"},
		{"./fixtures/swift/baseproj/Sources/BaseProj/types.swift", "func seq<S: Sequence>(s: S) -> Int where S.Element == Int {", "seq"},
		// sourcekit-lsp answers this initializer with a zero-width location that
		// SearchSnippet skips rather than fails on
		{"./fixtures/swift/baseproj/Sources/BaseProj/types.swift", "let d = String(describing: p)", "ranges"},
	}

	for _, tc := range cases {
		og, root, err := wspace.SearchSnippet(t.Context(), tc.file, tc.snippet)
		if !assert.NoError(t, err, tc.snippet) {
			continue
		}

		fn, ok := root.(*ast.ASTFuncExpression)
		if !assert.True(t, ok, "expected root to be the enclosing function for %q, got %T", tc.snippet, root) {
			continue
		}

		if assert.NotNil(t, fn.Name, tc.snippet) {
			assert.Equal(t, tc.root, fn.Name.Name, tc.snippet)
		}

		adj, err := og.Graph.AdjacencyMap()
		if assert.NoError(t, err) {
			assert.NotEmpty(t, adj, tc.snippet)
		}
	}
}

// A type declared in the same file resolves onto its own declaration
func TestSwiftSearchSnippetResolvesLocalAlias(t *testing.T) {
	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	wspace := cog.NewWorkspace(cwd)

	og, _, err := wspace.SearchSnippet(t.Context(), "./fixtures/swift/baseproj/Sources/BaseProj/types.swift", "func label(v: Label) -> Label {")
	if !assert.NoError(t, err) {
		return
	}

	pf := parseTestFile(t, "./fixtures/swift/baseproj/Sources/BaseProj/types.swift")

	adj, err := og.Graph.AdjacencyMap()
	if !assert.NoError(t, err) {
		return
	}

	// The alias has no members, so its vertex is the module that declares it
	assert.Contains(t, adj, pf.Module.GetHash(), "expected the module declaring the alias in the graph")
}
