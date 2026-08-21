package tests_test

import (
	"testing"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/stretchr/testify/assert"
)

func parseTSSimple(t *testing.T) *cog.COGFile {
	t.Helper()
	return parseTestFile(t, "./fixtures/typescript/baseproj/simple.ts")
}

func TestTSParserModule(t *testing.T) {
	pf := parseTSSimple(t)
	assert.Equal(t, "simple", pf.Module.Name)
	assert.Len(t, pf.Module.Block.Children(), 1)
}

func TestTSParserFunctionDeclaration(t *testing.T) {
	pf := parseTSSimple(t)
	fn, ok := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected ASTFuncExpression as first top-level node") {
		return
	}
	if assert.NotNil(t, fn.Name) {
		assert.Equal(t, "Symbol:x", fn.Name.String())
	}
	assert.Len(t, fn.Arguments, 1)

	// number and string are predefined types with no definition site, so
	// neither annotation becomes a type expression
	assert.Nil(t, fn.ReturnType)
}

func TestTSParserFunctionArgument(t *testing.T) {
	pf := parseTSSimple(t)
	fn := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.Len(t, fn.Arguments, 1) {
		return
	}
	arg := fn.Arguments[0]
	if assert.NotNil(t, arg.Identifier) {
		assert.Equal(t, "v", arg.Identifier.Name)
	}
	assert.Nil(t, arg.Type)
}

func TestTSParserFunctionBody(t *testing.T) {
	pf := parseTSSimple(t)
	fn := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.Len(t, fn.Block.Children(), 1) {
		return
	}
	_, ok := fn.Block.Children()[0].(*ast.ASTReturnStatement)
	assert.True(t, ok, "expected ASTReturnStatement as the only body statement")
}

func TestTSParserCallExpression(t *testing.T) {
	pf := parseTSSimple(t)
	fn := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)
	ret := fn.Block.Children()[0].(*ast.ASTReturnStatement)
	if !assert.Len(t, ret.Virtual, 1) {
		return
	}
	call, ok := ret.Virtual[0].(*ast.ASTCallExpression)
	if !assert.True(t, ok, "expected ASTCallExpression inside return statement") {
		return
	}
	assert.Nil(t, call.Namespace)
	if assert.NotNil(t, call.Symbol) {
		assert.Equal(t, "String", call.Symbol.Name)
	}
	if assert.Len(t, call.Arguments, 1) && assert.NotNil(t, call.Arguments[0].Identifier) {
		assert.Equal(t, "v", call.Arguments[0].Identifier.Name)
	}
}

func TestTSParserAttachesASTParents(t *testing.T) {
	pf := parseTSSimple(t)
	module := pf.Module
	fn := module.Block.Children()[0].(*ast.ASTFuncExpression)
	arg := fn.Arguments[0]
	ret := fn.Block.Children()[0].(*ast.ASTReturnStatement)
	call := ret.Virtual[0].(*ast.ASTCallExpression)

	assert.Nil(t, module.GetParent())
	assert.Same(t, module, module.Block.GetParent())
	assert.Same(t, module.Block, fn.GetParent())
	assert.Same(t, fn, fn.Name.GetParent())
	assert.Same(t, fn, arg.GetParent())
	assert.Same(t, arg, arg.Identifier.GetParent())
	assert.Same(t, fn, fn.Block.GetParent())
	assert.Same(t, fn.Block, ret.GetParent())
	assert.Same(t, ret, call.GetParent())
	assert.Same(t, call, call.Symbol.GetParent())
}

func TestTSParserClassAndMethod(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/typescript/baseproj/method.ts")

	widget := namedFunction(t, pf, "Widget")
	if widget == nil {
		return
	}

	members := widget.Block.Children()
	if !assert.Len(t, members, 2) {
		return
	}

	field, ok := members[0].(*ast.ASTDeclaration)
	if assert.True(t, ok, "expected a Declaration, got %T", members[0]) {
		if assert.Len(t, field.Names, 1) {
			assert.Equal(t, "id", field.Names[0].Name)
		}
	}

	describe, ok := members[1].(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected a FuncExpression, got %T", members[1]) {
		if assert.NotNil(t, describe.Name) {
			assert.Equal(t, "describe", describe.Name.Name)
		}
		if assert.Len(t, describe.Arguments, 1) && assert.NotNil(t, describe.Arguments[0].Identifier) {
			assert.Equal(t, "prefix", describe.Arguments[0].Identifier.Name)
		}
		assert.True(t, cog.IsNodeOfInterest(describe))
	}

	label := namedFunction(t, pf, "label")
	if label == nil || !assert.Len(t, label.Arguments, 1) {
		return
	}

	assertTypeSource(t, label.Arguments[0].Type, "Widget")

	// A definition resolves onto the method name, which must be the one node there
	nameRange, err := pf.FindSnippetRange([]byte("describe"))
	if !assert.NoError(t, err) {
		return
	}

	nodes := pf.FindNodesWithinRange(nameRange)
	if assert.Len(t, nodes, 1) {
		assert.Same(t, describe, nodes[0].NearestOrSelf(cog.IsNodeOfInterest))
	}
}

func TestTSParserInterface(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/typescript/baseproj/iface.ts")

	store := namedFunction(t, pf, "Store")
	if store == nil || !assert.Len(t, store.Block.Children(), 1) {
		return
	}

	get, ok := store.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected the method requirement, got %T", store.Block.Children()[0]) {
		return
	}

	if assert.NotNil(t, get.Name) {
		assert.Equal(t, "get", get.Name.Name)
	}
	assert.True(t, cog.IsNodeOfInterest(get))

	use := namedFunction(t, pf, "use")
	if use == nil || !assert.Len(t, use.Arguments, 1) {
		return
	}

	assertTypeSource(t, use.Arguments[0].Type, "Store")

	// A snippet starting at the `interface` keyword must root at the type,
	// not the module
	rng, err := pf.FindSnippetRange([]byte("interface Store {"))
	if !assert.NoError(t, err) {
		return
	}

	root := pf.FindTightestEnclosingNode(rng, cog.IsNodeOfInterest)
	assert.Same(t, store, root)
}

func TestTSParserImports(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/typescript/multidep/main.ts")

	chlds := pf.Module.Block.Children()
	if !assert.Len(t, chlds, 5) {
		return
	}

	fsImp, ok := chlds[0].(*ast.ASTImportStatement)
	if assert.True(t, ok, "expected an Import, got %T", chlds[0]) {
		assert.Nil(t, fsImp.Namespace)
		if assert.NotNil(t, fsImp.Reference) {
			assert.Equal(t, "node:fs", fsImp.Reference.Name)
		}
		assert.Equal(t, `"node:fs"`, fsImp.GetStringSource())
	}

	// Each named import binding carries a symbol of its own so a definition
	// answered with the local binding still resolves onto exactly one node
	binding, ok := chlds[1].(*ast.ASTSymbol)
	if assert.True(t, ok, "expected a Symbol for the import binding, got %T", chlds[1]) {
		assert.Equal(t, "readFileSync", binding.Name)
	}

	depImp, ok := chlds[2].(*ast.ASTImportStatement)
	if assert.True(t, ok, "expected an Import, got %T", chlds[2]) && assert.NotNil(t, depImp.Reference) {
		assert.Equal(t, "./pkg/dep1", depImp.Reference.Name)
	}

	alias, ok := chlds[3].(*ast.ASTSymbol)
	if assert.True(t, ok, "expected a Symbol for the import alias, got %T", chlds[3]) {
		assert.Equal(t, "fixtureDep1", alias.Name)
	}

	main, ok := chlds[4].(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected the exported function, got %T", chlds[4]) && assert.NotNil(t, main.Name) {
		assert.Equal(t, "main", main.Name.Name)
	}
}

func TestTSParserJavascriptDialect(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/typescript/baseproj/arrow.js")

	assert.Equal(t, "javascript", pf.Source.GetLanguage())

	decl, ok := pf.Module.Block.Children()[0].(*ast.ASTDeclaration)
	if !assert.True(t, ok, "expected a Declaration for the arrow const, got %T", pf.Module.Block.Children()[0]) {
		return
	}

	if assert.Len(t, decl.Names, 1) {
		assert.Equal(t, "double", decl.Names[0].Name)
	}

	if assert.Len(t, decl.Virtual, 1) {
		arrow, ok := decl.Virtual[0].(*ast.ASTFuncExpression)
		if assert.True(t, ok, "expected the arrow function, got %T", decl.Virtual[0]) {
			assert.Nil(t, arrow.Name)
			assert.Len(t, arrow.Arguments, 1)
		}
	}

	apply := namedFunction(t, pf, "apply")
	if apply == nil {
		return
	}

	calls := 0
	eachNode(apply, func(node ast.ASTNode) {
		if call, ok := node.(*ast.ASTCallExpression); ok && call.Symbol != nil {
			calls++
		}
	})

	assert.Equal(t, 2, calls, "expected values.map and double(v)")
}

func TestTSParserTSXDialect(t *testing.T) {
	pf := parseInline(t, "panel.tsx", "function Panel({ items }: Props) {\n  return <div>{items.map((item) => render(item))}</div>;\n}\n")
	if pf == nil {
		return
	}

	panel := namedFunction(t, pf, "Panel")
	if panel == nil {
		return
	}

	if assert.Len(t, panel.Arguments, 1) {
		if assert.NotNil(t, panel.Arguments[0].Identifier) {
			assert.Equal(t, "items", panel.Arguments[0].Identifier.Name)
		}
		assertTypeSource(t, panel.Arguments[0].Type, "Props")
	}

	rendered := false
	eachNode(panel, func(node ast.ASTNode) {
		if call, ok := node.(*ast.ASTCallExpression); ok && call.Symbol != nil && call.Symbol.Name == "render" {
			rendered = true
		}
	})

	assert.True(t, rendered, "expected the call inside the JSX expression to be mapped")
}

func TestTSParserRequireImport(t *testing.T) {
	pf := parseInline(t, "req.ts", "import handler = require(\"./handler\");\n")
	if pf == nil {
		return
	}

	imp, ok := pf.Module.Block.Children()[0].(*ast.ASTImportStatement)
	if !assert.True(t, ok, "expected an Import for the require clause, got %T", pf.Module.Block.Children()[0]) {
		return
	}

	if assert.NotNil(t, imp.Reference) {
		assert.Equal(t, "./handler", imp.Reference.Name)
	}
	if assert.NotNil(t, imp.Namespace) {
		assert.Equal(t, "handler", imp.Namespace.Name)
	}
}

func TestTSParserForOfBinding(t *testing.T) {
	pf := parseInline(t, "loop.ts", "function run(fns: Task[], pairs: Pair[]) {\n  for (const fn of fns) {\n    fn();\n  }\n  for (const [a, b] of pairs) {\n    a();\n  }\n}\n")
	if pf == nil {
		return
	}

	// A call of the loop variable resolves onto its binding, which must be
	// the one node there
	rng, err := pf.FindSnippetRange([]byte("fn of"))
	if !assert.NoError(t, err) {
		return
	}

	nodes := pf.FindNodesWithinRange(rng)
	if assert.Len(t, nodes, 1) {
		sym, ok := nodes[0].(*ast.ASTSymbol)
		if assert.True(t, ok, "expected the binding symbol, got %T", nodes[0]) {
			assert.Equal(t, "fn", sym.Name)
		}
	}

	decl := tsDeclarationNamed(t, pf, "a")
	if decl != nil && assert.Len(t, decl.Names, 2) {
		assert.Equal(t, "a", decl.Names[0].Name)
		assert.Equal(t, "b", decl.Names[1].Name)
	}
}

func TestTSParserCallWithoutCalleeIsNotEmitted(t *testing.T) {
	pf := parseInline(t, "nocallee.ts", "function z() {\n  (() => { g(); })();\n  fns[i]();\n  (f)();\n}\n")
	if pf == nil {
		return
	}

	calls := 0
	lambdas := 0

	eachNode(pf.Module, func(node ast.ASTNode) {
		switch n := node.(type) {
		case *ast.ASTCallExpression:
			calls++
			assert.NotNil(t, n.Symbol, "a call must carry a callee symbol")
		case *ast.ASTFuncExpression:
			if n.Name == nil {
				lambdas++
			}
		}
	})

	assert.Equal(t, 1, calls, "only g() has an identifiable callee")
	assert.Equal(t, 1, lambdas, "the IIFE body still indexes under z")
}
