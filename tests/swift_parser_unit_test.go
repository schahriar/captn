package tests_test

import (
	"os"
	"testing"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/common"
	"github.com/stretchr/testify/assert"
)

func parseSwiftSimple(t *testing.T) *cog.COGFile {
	t.Helper()
	return parseTestFile(t, "./fixtures/swift/baseproj/Sources/BaseProj/simple.swift")
}

func parseSwiftInline(t *testing.T, source string) *cog.COGFile {
	t.Helper()

	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return nil
	}

	pf, err := cog.ParseSource(t.Context(), common.NewSource(cwd, "inline.swift", []byte(source)))
	if !assert.NoError(t, err, "expected %q to parse", source) {
		return nil
	}

	return pf
}

// declaredNames collects every name introduced anywhere under the given node,
// whether it was mapped to a declaration or to a function
func declaredNames(node ast.ASTNode) []string {
	names := []string{}

	switch n := node.(type) {
	case *ast.ASTDeclaration:
		for _, name := range n.Names {
			names = append(names, name.Name)
		}
	case *ast.ASTFuncExpression:
		if n.Name != nil {
			names = append(names, n.Name.Name)
		}
	}

	for _, child := range node.Children() {
		names = append(names, declaredNames(child)...)
	}

	return names
}

func TestSwiftParserModule(t *testing.T) {
	pf := parseSwiftSimple(t)
	assert.Equal(t, "simple", pf.Module.Name)
	assert.Len(t, pf.Module.Block.Children(), 1)
}

func TestSwiftParserFunctionDeclaration(t *testing.T) {
	pf := parseSwiftSimple(t)

	fn, ok := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected ASTFuncExpression as first top-level node") {
		return
	}

	if assert.NotNil(t, fn.Name) {
		assert.Equal(t, "x", fn.Name.Name)
	}

	assert.Len(t, fn.Arguments, 1)

	if assert.NotNil(t, fn.ReturnType) {
		assert.Equal(t, "String", fn.ReturnType.Name)
	}
}

func TestSwiftParserFunctionArgument(t *testing.T) {
	pf := parseSwiftSimple(t)
	fn := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)

	if !assert.Len(t, fn.Arguments, 1) {
		return
	}

	arg := fn.Arguments[0]

	if assert.NotNil(t, arg.Identifier) {
		assert.Equal(t, "v", arg.Identifier.Name)
	}

	if assert.NotNil(t, arg.Type) {
		assert.Equal(t, "Int", arg.Type.Name)
	}
}

func TestSwiftParserFunctionBody(t *testing.T) {
	// Swift's return maps to control_transfer_statement, which is deliberately
	// unmapped; the call inside it lands directly in the function block
	pf := parseSwiftSimple(t)
	fn := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)

	if !assert.Len(t, fn.Block.Children(), 1) {
		return
	}

	call, ok := fn.Block.Children()[0].(*ast.ASTCallExpression)
	if !assert.True(t, ok, "expected ASTCallExpression as the only body statement") {
		return
	}

	assert.Nil(t, call.Namespace)

	if assert.NotNil(t, call.Symbol) {
		assert.Equal(t, "String", call.Symbol.Name)
	}
}

func TestSwiftParserStructDeclaration(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/swift/baseproj/Sources/BaseProj/method.swift")

	widget, ok := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected the struct to map to ASTFuncExpression") {
		return
	}

	if assert.NotNil(t, widget.Name) {
		assert.Equal(t, "Widget", widget.Name.Name)
	}

	if !assert.Len(t, widget.Block.Children(), 1) {
		return
	}

	method, ok := widget.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected the method inside the struct block") {
		return
	}

	if assert.NotNil(t, method.Name) {
		assert.Equal(t, "describe", method.Name.Name)
	}

	assert.Len(t, method.Arguments, 1)
}

func TestSwiftParserImports(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/swift/multidep/Sources/App/main.swift")

	refs := []string{}

	for _, chld := range pf.Module.Block.Children() {
		if imp, ok := chld.(*ast.ASTImportStatement); ok && imp.Reference != nil {
			refs = append(refs, imp.Reference.Name)
		}
	}

	assert.Equal(t, []string{"Foundation", "Dep1"}, refs)
}

func TestSwiftParserChainedCall(t *testing.T) {
	// print(getExampleText().uppercased()) nests three calls; each inner call
	// must land in the enclosing call's virtual children
	pf := parseTestFile(t, "./fixtures/swift/multidep/Sources/App/main.swift")

	var fn *ast.ASTFuncExpression

	for _, chld := range pf.Module.Block.Children() {
		if f, ok := chld.(*ast.ASTFuncExpression); ok {
			fn = f
		}
	}

	if !assert.NotNil(t, fn, "expected the main function") {
		return
	}

	if !assert.Len(t, fn.Block.Children(), 1) {
		return
	}

	outer, ok := fn.Block.Children()[0].(*ast.ASTCallExpression)
	if !assert.True(t, ok) {
		return
	}

	assert.Equal(t, "print", outer.Symbol.Name)

	if !assert.Len(t, outer.Virtual, 1) {
		return
	}

	middle, ok := outer.Virtual[0].(*ast.ASTCallExpression)
	if !assert.True(t, ok, "expected the uppercased() call inside print") {
		return
	}

	assert.Equal(t, "uppercased", middle.Symbol.Name)
	assert.Nil(t, middle.Namespace, "the receiver is a call, not a plain identifier")

	if !assert.Len(t, middle.Virtual, 1) {
		return
	}

	inner, ok := middle.Virtual[0].(*ast.ASTCallExpression)
	if !assert.True(t, ok, "expected the getExampleText() call as the receiver") {
		return
	}

	assert.Equal(t, "getExampleText", inner.Symbol.Name)
}

func TestSwiftParserNamespacedCall(t *testing.T) {
	pf := parseSwiftInline(t, "func go(w: Widget) {\n    w.describe(prefix: \"a\")\n}\n")
	if pf == nil {
		return
	}

	fn := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)

	if !assert.Len(t, fn.Block.Children(), 1) {
		return
	}

	call, ok := fn.Block.Children()[0].(*ast.ASTCallExpression)
	if !assert.True(t, ok) {
		return
	}

	if assert.NotNil(t, call.Namespace) {
		assert.Equal(t, "w", call.Namespace.Name)
	}

	if assert.NotNil(t, call.Symbol) {
		assert.Equal(t, "describe", call.Symbol.Name)
	}
}

func TestSwiftParserInitDeclaration(t *testing.T) {
	pf := parseSwiftInline(t, "struct A {\n    init(v: Int) {\n        setup(v)\n    }\n}\n")
	if pf == nil {
		return
	}

	a := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)

	if !assert.Len(t, a.Block.Children(), 1) {
		return
	}

	ctor, ok := a.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected the initializer to map to ASTFuncExpression") {
		return
	}

	// Constructor-call definitions resolve onto the init keyword
	if assert.NotNil(t, ctor.Name) {
		assert.Equal(t, "init", ctor.Name.Name)
	}

	assert.Len(t, ctor.Arguments, 1)
}

func TestSwiftParserExtensionAndProtocol(t *testing.T) {
	pf := parseSwiftInline(t, "extension Widget {\n    func extra() {}\n}\n\nprotocol Greeter {\n    func greet(name: String) -> String\n}\n")
	if pf == nil {
		return
	}

	if !assert.Len(t, pf.Module.Block.Children(), 2) {
		return
	}

	ext, ok := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected the extension to map to ASTFuncExpression") {
		return
	}

	// An extension names a type declared elsewhere; the symbol still anchors the vertex
	if assert.NotNil(t, ext.Name) {
		assert.Equal(t, "Widget", ext.Name.Name)
	}

	proto, ok := pf.Module.Block.Children()[1].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected the protocol to map to ASTFuncExpression") {
		return
	}

	if assert.NotNil(t, proto.Name) {
		assert.Equal(t, "Greeter", proto.Name.Name)
	}

	// A protocol requirement is a function with no body. It still maps to
	// FuncExpression so it can be an observation vertex in its own right
	if assert.Len(t, proto.Block.Children(), 1) {
		req, ok := proto.Block.Children()[0].(*ast.ASTFuncExpression)
		if assert.True(t, ok, "expected the requirement to map to ASTFuncExpression") {
			assert.Equal(t, "greet", req.Name.Name)
		}
	}
}

func TestSwiftParserEnumCasesDeclare(t *testing.T) {
	pf := parseSwiftInline(t, "enum Kind {\n    case simple\n    case tagged(String)\n}\n")
	if pf == nil {
		return
	}

	kind, ok := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected the enum to map to ASTFuncExpression") {
		return
	}

	names := declaredNames(kind)
	assert.Contains(t, names, "simple")
	assert.Contains(t, names, "tagged")
}

func TestSwiftParserPropertyDeclaration(t *testing.T) {
	pf := parseSwiftInline(t, "let handler = makeHandler()\n")
	if pf == nil {
		return
	}

	decl, ok := pf.Module.Block.Children()[0].(*ast.ASTDeclaration)
	if !assert.True(t, ok, "expected ASTDeclaration for the property") {
		return
	}

	if assert.Len(t, decl.Names, 1) {
		assert.Equal(t, "handler", decl.Names[0].Name)
	}

	if !assert.Len(t, decl.Virtual, 1) {
		return
	}

	call, ok := decl.Virtual[0].(*ast.ASTCallExpression)
	if assert.True(t, ok, "expected the right-hand side call in Virtual") {
		assert.Equal(t, "makeHandler", call.Symbol.Name)
	}
}

func TestSwiftParserLambda(t *testing.T) {
	pf := parseSwiftInline(t, "let f = { (v: Int) in String(v) }\n")
	if pf == nil {
		return
	}

	decl, ok := pf.Module.Block.Children()[0].(*ast.ASTDeclaration)
	if !assert.True(t, ok, "expected ASTDeclaration for the assignment") {
		return
	}

	if !assert.Len(t, decl.Virtual, 1) {
		return
	}

	fn, ok := decl.Virtual[0].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected ASTFuncExpression for the closure") {
		return
	}

	assert.Nil(t, fn.Name)

	if assert.Len(t, fn.Arguments, 1) {
		assert.Equal(t, "v", fn.Arguments[0].Identifier.Name)
	}

	if assert.Len(t, fn.Block.Children(), 1) {
		_, ok := fn.Block.Children()[0].(*ast.ASTCallExpression)
		assert.True(t, ok, "expected ASTCallExpression as the closure body")
	}
}

func TestSwiftParserBindingFormsDeclare(t *testing.T) {
	cases := map[string]string{
		"func go(g: () -> Foo?) {\n    if let n = g() {\n        n.run()\n    }\n}\n":                        "n",
		"func go(g: () -> Foo?) {\n    guard let m = g() else { return }\n    m.run()\n}\n":                  "m",
		"func go(xs: [Foo]) {\n    for item in xs {\n        item.use()\n    }\n}\n":                         "item",
		"func go() {\n    do {\n        try risky()\n    } catch let err {\n        report(err)\n    }\n}\n": "err",
	}

	for source, want := range cases {
		pf := parseSwiftInline(t, source)
		if pf == nil {
			continue
		}

		assert.Contains(t, declaredNames(pf.Module), want, "expected a declaration binding %q in %q", want, source)
	}
}

func TestSwiftParserBodylessDeclarationsDeclare(t *testing.T) {
	// A function without a body is only legal inside a protocol, so the grammar
	// has no production for one anywhere else and error recovery takes over.
	// Every declaration in a .swiftinterface has this shape, as does any
	// declaration being typed, and the name still has to carry a symbol or a
	// definition resolving onto it finds nothing.
	cases := map[string]string{
		"public func topLevel() -> Int\n":                  "topLevel",
		"extension A {\n    public func g() -> Int\n}\n":   "g",
		"protocol P {\n    func requirement() -> Int\n}\n": "requirement",
		"protocol P {\n    var value: Int { get }\n}\n":    "value",
	}

	for source, want := range cases {
		pf := parseSwiftInline(t, source)
		if pf == nil {
			continue
		}

		assert.Contains(t, declaredNames(pf.Module), want, "expected %q to be declared in %q", want, source)
	}
}

func TestSwiftParserErrorRecoveryKeepsTypesOut(t *testing.T) {
	// The recovered ERROR node carries the return type under a second name
	// field; it is spelled user_type, so only the declaration name is collected
	pf := parseSwiftInline(t, "public func topLevel() -> Widget\n")
	if pf == nil {
		return
	}

	names := declaredNames(pf.Module)
	assert.Contains(t, names, "topLevel")
	assert.NotContains(t, names, "Widget")
}

func TestSwiftParserAttachesASTParents(t *testing.T) {
	pf := parseSwiftSimple(t)

	module := pf.Module
	fn := module.Block.Children()[0].(*ast.ASTFuncExpression)
	arg := fn.Arguments[0]
	call := fn.Block.Children()[0].(*ast.ASTCallExpression)

	assert.Nil(t, module.GetParent())
	assert.Same(t, module, module.Block.GetParent())
	assert.Same(t, module.Block, fn.GetParent())
	assert.Same(t, fn, fn.Name.GetParent())
	assert.Same(t, fn, arg.GetParent())
	assert.Same(t, arg, arg.Identifier.GetParent())
	assert.Same(t, arg, arg.Type.GetParent())
	assert.Same(t, fn, fn.ReturnType.GetParent())
	assert.Same(t, fn, fn.Block.GetParent())
	assert.Same(t, fn.Block, call.GetParent())
	assert.Same(t, call, call.Symbol.GetParent())
}

func TestSwiftParserBrokenBodyStillParses(t *testing.T) {
	// tree-sitter error recovery emits zero-width nodes for half-written
	// declarations; parsing and indexing must survive them
	for _, source := range []string{
		"func f() {\n",
		"struct A {\n",
		"func f(\n",
		"extension {\n}\n",
		"func go(xs: [Foo]) {\n    for  in xs {\n    }\n}\n",
		"func go() {\n    if let  = x {\n    }\n}\n",
	} {
		if pf := parseSwiftInline(t, source); pf != nil {
			assert.NotNil(t, pf.Module)
		}
	}
}

func TestParserSwiftSimpleParse(t *testing.T) {
	checkSnapshot(t, "./fixtures/swift/baseproj/Sources/BaseProj/simple.swift")
}

func TestParserSwiftMultiDepParse(t *testing.T) {
	checkSnapshot(t, "./fixtures/swift/multidep/Sources/App/main.swift")
}
