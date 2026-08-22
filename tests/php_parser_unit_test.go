package tests_test

import (
	"os"
	"testing"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/common"
	"github.com/stretchr/testify/assert"
)

func parsePhpSimple(t *testing.T) *cog.COGFile {
	t.Helper()
	return parseTestFile(t, "./fixtures/php/baseproj/simple.php")
}

func parsePhpInline(t *testing.T, source string) *cog.COGFile {
	t.Helper()
	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return nil
	}
	pf, err := cog.ParseSource(t.Context(), common.NewSource(cwd, "inline.php", []byte(source)))
	if !assert.NoError(t, err, "expected %q to parse", source) {
		return nil
	}
	return pf
}

func TestPHPParserModule(t *testing.T) {
	pf := parsePhpSimple(t)
	assert.Equal(t, "simple", pf.Module.Name)
	assert.Len(t, pf.Module.Block.Children(), 1)
}

func TestPHPParserNamespaceNamesModule(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/php/multidep/main.php")
	assert.Equal(t, "App", pf.Module.Name)
}

func TestPHPParserFunctionDefinition(t *testing.T) {
	pf := parsePhpSimple(t)
	fn, ok := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected ASTFuncExpression as first top-level node") {
		return
	}
	if assert.NotNil(t, fn.Name) {
		assert.Equal(t, "Symbol:x", fn.Name.String())
	}
	if assert.NotNil(t, fn.ReturnType) {
		assert.Equal(t, "string", fn.ReturnType.Name)
	}
	if assert.Len(t, fn.Arguments, 1) {
		arg := fn.Arguments[0]
		if assert.NotNil(t, arg.Identifier) {
			assert.Equal(t, "$v", arg.Identifier.Name)
		}
		if assert.NotNil(t, arg.Type) {
			assert.Equal(t, "int", arg.Type.Name)
		}
	}
}

func TestPHPParserFunctionBody(t *testing.T) {
	// Return statements are deliberately unmapped; the call inside the
	// return lands directly in the function block
	pf := parsePhpSimple(t)
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
		assert.Equal(t, "strval", call.Symbol.Name)
	}
}

func TestPHPParserClassDefinition(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/php/baseproj/method.php")
	class, ok := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected the class to map to ASTFuncExpression") {
		return
	}
	if assert.NotNil(t, class.Name) {
		assert.Equal(t, "Widget", class.Name.Name)
	}
	if !assert.Len(t, class.Block.Children(), 3) {
		return
	}

	prop, ok := class.Block.Children()[0].(*ast.ASTDeclaration)
	if assert.True(t, ok, "expected ASTDeclaration for the property") && assert.Len(t, prop.Names, 1) {
		assert.Equal(t, "$label", prop.Names[0].Name)
		if assert.NotNil(t, prop.Type) {
			assert.Equal(t, "string", prop.Type.Name)
		}
	}

	ctor, ok := class.Block.Children()[1].(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected the constructor") {
		assert.Equal(t, "__construct", ctor.Name.Name)
	}

	describe, ok := class.Block.Children()[2].(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected the describe method") {
		assert.Equal(t, "describe", describe.Name.Name)
		assert.Len(t, describe.Arguments, 1)
		if assert.Len(t, describe.Block.Children(), 1) {
			call, ok := describe.Block.Children()[0].(*ast.ASTCallExpression)
			if assert.True(t, ok, "expected the sprintf call") {
				assert.Equal(t, "sprintf", call.Symbol.Name)
			}
		}
	}
}

func TestPHPParserInterfaceAndClauses(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/php/baseproj/types.php")

	iface, ok := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected the interface as ASTFuncExpression") {
		return
	}
	assert.Equal(t, "Renderer", iface.Name.Name)
	if assert.Len(t, iface.Block.Children(), 1) {
		render, ok := iface.Block.Children()[0].(*ast.ASTFuncExpression)
		if assert.True(t, ok, "expected the bodyless interface method") {
			assert.Equal(t, "render", render.Name.Name)
		}
	}

	card, ok := pf.Module.Block.Children()[2].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected the Card class") {
		return
	}
	assert.Equal(t, "Card", card.Name.Name)

	var clauses []string
	for _, chld := range card.Block.Children() {
		if texpr, ok := chld.(*ast.ASTTypeExpression); ok {
			clauses = append(clauses, texpr.Name)
		}
	}
	assert.Equal(t, []string{"Base", "Renderer"}, clauses)
}

func TestPHPParserArrowFunction(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/php/baseproj/types.php")

	decl, ok := pf.Module.Block.Children()[4].(*ast.ASTDeclaration)
	if !assert.True(t, ok, "expected ASTDeclaration for the assignment") {
		return
	}
	if assert.Len(t, decl.Names, 1) {
		assert.Equal(t, "$make_card", decl.Names[0].Name)
	}
	if !assert.Len(t, decl.Virtual, 1) {
		return
	}
	fn, ok := decl.Virtual[0].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected ASTFuncExpression for the arrow function") {
		return
	}
	assert.Nil(t, fn.Name)
	if assert.Len(t, fn.Arguments, 1) {
		assert.Equal(t, "$label", fn.Arguments[0].Identifier.Name)
	}
	if assert.NotNil(t, fn.ReturnType) {
		assert.Equal(t, "Card", fn.ReturnType.Name)
	}
	if assert.Len(t, fn.Block.Children(), 1) {
		call, ok := fn.Block.Children()[0].(*ast.ASTCallExpression)
		if assert.True(t, ok, "expected the constructor call as the arrow body") {
			assert.Equal(t, "Card", call.Symbol.Name)
		}
	}
}

func TestPHPParserImports(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/php/multidep/main.php")
	children := pf.Module.Block.Children()
	if !assert.GreaterOrEqual(t, len(children), 3) {
		return
	}

	stdlib, ok := children[0].(*ast.ASTImportStatement)
	if assert.True(t, ok, "expected ASTImportStatement for use DateTime") {
		assert.Equal(t, "DateTime", stdlib.Reference.Name)
	}

	local, ok := children[1].(*ast.ASTImportStatement)
	if assert.True(t, ok, "expected ASTImportStatement for the aliased use") {
		assert.Equal(t, "App\\Dep1", local.Reference.Name)
		if assert.NotNil(t, local.Namespace) {
			assert.Equal(t, "FixtureDep1", local.Namespace.Name)
		}
	}

	req, ok := children[2].(*ast.ASTImportStatement)
	if assert.True(t, ok, "expected ASTImportStatement for require_once") {
		assert.Equal(t, "dep1.php", req.Reference.Name)
	}
}

func TestPHPParserScopedCall(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/php/multidep/main.php")
	fn, ok := pf.Module.Block.Children()[3].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected the run function") {
		return
	}
	if !assert.Len(t, fn.Block.Children(), 2) {
		return
	}

	decl, ok := fn.Block.Children()[0].(*ast.ASTDeclaration)
	if assert.True(t, ok, "expected ASTDeclaration for $now") {
		if assert.Len(t, decl.Names, 1) {
			assert.Equal(t, "$now", decl.Names[0].Name)
		}
		if assert.Len(t, decl.Virtual, 1) {
			call, ok := decl.Virtual[0].(*ast.ASTCallExpression)
			if assert.True(t, ok, "expected the object creation as a call") {
				assert.Equal(t, "DateTime", call.Symbol.Name)
			}
		}
	}

	call, ok := fn.Block.Children()[1].(*ast.ASTCallExpression)
	if assert.True(t, ok, "expected the scoped call") {
		assert.Equal(t, "exampleText", call.Symbol.Name)
		if assert.NotNil(t, call.Namespace) {
			assert.Equal(t, "FixtureDep1", call.Namespace.Name)
		}
	}
}

func TestPHPParserAttachesASTParents(t *testing.T) {
	pf := parsePhpSimple(t)
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

func TestPHPParserBrokenBodyStillParses(t *testing.T) {
	// tree-sitter error recovery emits zero-width nodes for half-written
	// definitions; parsing and indexing must survive them
	for _, source := range []string{
		"<?php function f(\n",
		"<?php class Widget { public function \n}\n",
		"<?php $x = \n",
		"<?php use ;\n",
		"plain html, no php tag at all\n",
	} {
		if pf := parsePhpInline(t, source); pf != nil {
			assert.NotNil(t, pf.Module)
		}
	}
}

func TestParserPHPSimpleParse(t *testing.T) {
	checkSnapshot(t, "./fixtures/php/baseproj/simple.php")
}

func TestParserPHPTypesParse(t *testing.T) {
	checkSnapshot(t, "./fixtures/php/baseproj/types.php")
}

func TestParserPHPMultiDepParse(t *testing.T) {
	checkSnapshot(t, "./fixtures/php/multidep/main.php")
}
