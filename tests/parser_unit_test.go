package tests_test

import (
	"os"
	"testing"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/stretchr/testify/assert"
)

func parseSimple(t *testing.T) *cog.COGNode {
	t.Helper()
	cwd, err := os.Getwd()
	assert.NoError(t, err)
	pf, err := cog.ParseFile(t.Context(), cwd, "./fixtures/golang/baseproj/simple.go")
	assert.NoError(t, err)
	return pf
}

func TestParserModule(t *testing.T) {
	pf := parseSimple(t)
	assert.Equal(t, "fixture_main", pf.Module.Name)
	assert.Len(t, pf.Module.Block.Children(), 1)
}

func TestParserFunctionDeclaration(t *testing.T) {
	pf := parseSimple(t)
	fn, ok := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected ASTFuncExpression as first top-level node") {
		return
	}
	if assert.NotNil(t, fn.Name) {
		assert.Equal(t, "Symbol:x", fn.Name.String())
	}
	assert.Len(t, fn.Arguments, 1)
	if assert.NotNil(t, fn.ReturnType) {
		assert.Equal(t, "string", fn.ReturnType.Name)
	}
}

func TestParserFunctionArgument(t *testing.T) {
	pf := parseSimple(t)
	fn := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.Len(t, fn.Arguments, 1) {
		return
	}
	arg := fn.Arguments[0]
	if assert.NotNil(t, arg.Identifier) {
		assert.Equal(t, "v", arg.Identifier.Name)
	}
	if assert.NotNil(t, arg.Type) {
		assert.Equal(t, "int", arg.Type.Name)
	}
}

func TestParserFunctionBody(t *testing.T) {
	pf := parseSimple(t)
	fn := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.Len(t, fn.Block.Children(), 1) {
		return
	}
	_, ok := fn.Block.Children()[0].(*ast.ASTReturnStatement)
	assert.True(t, ok, "expected ASTReturnStatement as the only body statement")
}

func TestParserReturnStatement(t *testing.T) {
	pf := parseSimple(t)
	fn := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)
	ret := fn.Block.Children()[0].(*ast.ASTReturnStatement)
	if !assert.Len(t, ret.Virtual, 1) {
		return
	}
	_, ok := ret.Virtual[0].(*ast.ASTCallExpression)
	assert.True(t, ok, "expected ASTCallExpression inside return statement")
}

func TestParserCallExpression(t *testing.T) {
	pf := parseSimple(t)
	fn := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)
	ret := fn.Block.Children()[0].(*ast.ASTReturnStatement)
	call := ret.Virtual[0].(*ast.ASTCallExpression)
	assert.Nil(t, call.Namespace)
	if assert.NotNil(t, call.Symbol) {
		assert.Equal(t, "string", call.Symbol.Name)
	}
	if assert.Len(t, call.Arguments, 1) {
		assert.Nil(t, call.Arguments[0].Identifier)
	}
}
