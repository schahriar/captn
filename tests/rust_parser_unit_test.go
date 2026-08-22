package tests_test

import (
	"testing"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/stretchr/testify/assert"
)

func parseRsSimple(t *testing.T) *cog.COGFile {
	t.Helper()
	return parseTestFile(t, "./fixtures/rust/baseproj/src/simple.rs")
}

func TestRustParserModule(t *testing.T) {
	pf := parseRsSimple(t)
	assert.Equal(t, "simple", pf.Module.Name)
	assert.Len(t, pf.Module.Block.Children(), 2)
}

func TestRustParserFunctionDeclaration(t *testing.T) {
	pf := parseRsSimple(t)
	fn, ok := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected ASTFuncExpression as first top-level node") {
		return
	}
	if assert.NotNil(t, fn.Name) {
		assert.Equal(t, "Symbol:x", fn.Name.String())
	}
	assert.Len(t, fn.Arguments, 1)
	if assert.NotNil(t, fn.ReturnType) {
		assert.Equal(t, "String", fn.ReturnType.Name)
	}
}

func TestRustParserFunctionArgument(t *testing.T) {
	pf := parseRsSimple(t)
	fn := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.Len(t, fn.Arguments, 1) {
		return
	}
	arg := fn.Arguments[0]
	if assert.NotNil(t, arg.Identifier) {
		assert.Equal(t, "v", arg.Identifier.Name)
	}

	// A primitive type has no definition site and maps to no type expression
	assert.Nil(t, arg.Type)
}

func TestRustParserFunctionBody(t *testing.T) {
	pf := parseRsSimple(t)
	fn := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.Len(t, fn.Block.Children(), 1) {
		return
	}
	_, ok := fn.Block.Children()[0].(*ast.ASTCallExpression)
	assert.True(t, ok, "expected ASTCallExpression as the only body statement")
}

func TestRustParserCallExpression(t *testing.T) {
	pf := parseRsSimple(t)
	fn := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)
	call := fn.Block.Children()[0].(*ast.ASTCallExpression)
	assert.Nil(t, call.Namespace)
	if assert.NotNil(t, call.Symbol) {
		assert.Equal(t, "stringify", call.Symbol.Name)
	}
}

func TestRustParserAttachesASTParents(t *testing.T) {
	pf := parseRsSimple(t)
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
	assert.Same(t, fn, fn.ReturnType.GetParent())
	assert.Same(t, fn, fn.Block.GetParent())
	assert.Same(t, fn.Block, call.GetParent())
	assert.Same(t, call, call.Symbol.GetParent())
}

func TestRustParserMethodCall(t *testing.T) {
	pf := parseRsSimple(t)
	fn := pf.Module.Block.Children()[1].(*ast.ASTFuncExpression)
	if !assert.NotNil(t, fn.Name) || !assert.Equal(t, "stringify", fn.Name.Name) {
		return
	}
	if !assert.Len(t, fn.Block.Children(), 1) {
		return
	}
	call, ok := fn.Block.Children()[0].(*ast.ASTCallExpression)
	if !assert.True(t, ok, "expected ASTCallExpression for the method call") {
		return
	}
	if assert.NotNil(t, call.Symbol) {
		assert.Equal(t, "to_string", call.Symbol.Name)
	}
	if assert.NotNil(t, call.Namespace) {
		assert.Equal(t, "v", call.Namespace.Name)
	}
}

func TestRustParserStructAndImpl(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/rust/baseproj/src/method.rs")

	widget := namedFunction(t, pf, "Widget")
	if widget == nil {
		return
	}

	fields := widget.Block.Children()
	if !assert.Len(t, fields, 1) {
		return
	}

	label, ok := fields[0].(*ast.ASTDeclaration)
	if assert.True(t, ok, "expected a Declaration, got %T", fields[0]) {
		if assert.Len(t, label.Names, 1) {
			assert.Equal(t, "label", label.Names[0].Name)
		}
		assertTypeSource(t, label.Type, "String")
	}

	// The impl is a second vertex named after its self type, holding the method
	var impl *ast.ASTFuncExpression
	for _, chld := range pf.Module.Block.Children() {
		fn, ok := chld.(*ast.ASTFuncExpression)
		if ok && fn != widget {
			impl = fn
		}
	}

	if !assert.NotNil(t, impl, "expected the impl block as its own FuncExpression") {
		return
	}

	if assert.NotNil(t, impl.Name) {
		assert.Equal(t, "Widget", impl.Name.Name)
	}

	if !assert.Len(t, impl.Block.Children(), 1) {
		return
	}

	describe, ok := impl.Block.Children()[0].(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected the method, got %T", impl.Block.Children()[0]) {
		if assert.NotNil(t, describe.Name) {
			assert.Equal(t, "describe", describe.Name.Name)
		}
		assert.True(t, cog.IsNodeOfInterest(describe))
	}
}

func TestRustParserTraitRequirement(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/rust/baseproj/src/traits.rs")

	store := namedFunction(t, pf, "Store")
	if store == nil || !assert.Len(t, store.Block.Children(), 1) {
		return
	}

	get, ok := store.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected the bodyless requirement, got %T", store.Block.Children()[0]) {
		return
	}

	if assert.NotNil(t, get.Name) {
		assert.Equal(t, "get", get.Name.Name)
	}
	assertTypeSource(t, get.ReturnType, "String")
	assert.True(t, cog.IsNodeOfInterest(get))

	// A definition resolves onto the requirement name, which must be the one
	// node there
	nameRange, err := pf.FindSnippetRange([]byte("get"))
	if !assert.NoError(t, err) {
		return
	}

	nodes := pf.FindNodesWithinRange(nameRange)
	if assert.Len(t, nodes, 1) {
		assert.Same(t, get, nodes[0].NearestOrSelf(cog.IsNodeOfInterest))
	}
}

func TestRustParserImports(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/rust/multidep/src/main.rs")

	impVis := ast.NewImportVisitor()
	pf.Module.Accept(impVis)

	if !assert.Len(t, impVis.Imports, 3) {
		return
	}

	refs := map[string]string{}

	for _, imp := range impVis.Imports {
		if !assert.NotNil(t, imp.Reference) {
			continue
		}

		ns := ""
		if imp.Namespace != nil {
			ns = imp.Namespace.Name
		}

		refs[imp.Reference.Name] = ns
	}

	assert.Equal(t, map[string]string{
		"std::collections::HashMap": "",
		"dep1":                      "",
		"dep1::get_example_text":    "example_text",
	}, refs)
}

func TestRustParserClosure(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/rust/baseproj/src/deferred.rs")

	cleanup := namedFunction(t, pf, "cleanup")
	if cleanup == nil {
		return
	}

	decl, ok := cleanup.Block.Children()[0].(*ast.ASTDeclaration)
	if !assert.True(t, ok, "expected the let binding, got %T", cleanup.Block.Children()[0]) {
		return
	}

	if assert.Len(t, decl.Names, 1) {
		assert.Equal(t, "done", decl.Names[0].Name)
	}

	if !assert.Len(t, decl.Virtual, 1) {
		return
	}

	closure, ok := decl.Virtual[0].(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected the closure under the declaration, got %T", decl.Virtual[0]) {
		assert.Nil(t, closure.Name)
	}
}
