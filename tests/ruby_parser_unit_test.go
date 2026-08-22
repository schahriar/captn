package tests_test

import (
	"os"
	"testing"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/common"
	"github.com/stretchr/testify/assert"
)

func parseRbSimple(t *testing.T) *cog.COGFile {
	t.Helper()
	return parseTestFile(t, "./fixtures/ruby/baseproj/simple.rb")
}

func parseRbInline(t *testing.T, source string) *cog.COGFile {
	t.Helper()
	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return nil
	}
	pf, err := cog.ParseSource(t.Context(), common.NewSource(cwd, "inline.rb", []byte(source)))
	if !assert.NoError(t, err, "expected %q to parse", source) {
		return nil
	}
	return pf
}

func TestRubyParserModule(t *testing.T) {
	pf := parseRbSimple(t)
	assert.Equal(t, "simple", pf.Module.Name)
	assert.Len(t, pf.Module.Block.Children(), 1)
}

func TestRubyParserMethodDefinition(t *testing.T) {
	pf := parseRbSimple(t)
	fn, ok := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected ASTFuncExpression as first top-level node") {
		return
	}
	if assert.NotNil(t, fn.Name) {
		assert.Equal(t, "Symbol:x", fn.Name.String())
	}
	if assert.Len(t, fn.Arguments, 1) {
		arg := fn.Arguments[0]
		if assert.NotNil(t, arg.Identifier) {
			assert.Equal(t, "v", arg.Identifier.Name)
		}
		assert.Nil(t, arg.Type)
	}
}

func TestRubyParserMethodBody(t *testing.T) {
	pf := parseRbSimple(t)
	fn := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.Len(t, fn.Block.Children(), 1) {
		return
	}
	call, ok := fn.Block.Children()[0].(*ast.ASTCallExpression)
	if !assert.True(t, ok, "expected ASTCallExpression as the only body statement") {
		return
	}
	if assert.NotNil(t, call.Namespace) {
		assert.Equal(t, "v", call.Namespace.Name)
	}
	if assert.NotNil(t, call.Symbol) {
		assert.Equal(t, "to_s", call.Symbol.Name)
	}
}

func TestRubyParserClassDefinition(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/ruby/baseproj/method.rb")
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

	attr, ok := class.Block.Children()[0].(*ast.ASTDeclaration)
	if assert.True(t, ok, "expected ASTDeclaration for attr_reader") && assert.Len(t, attr.Names, 1) {
		assert.Equal(t, "label", attr.Names[0].Name)
	}

	init, ok := class.Block.Children()[1].(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected the initialize method") {
		assert.Equal(t, "initialize", init.Name.Name)
		if assert.Len(t, init.Block.Children(), 1) {
			decl, ok := init.Block.Children()[0].(*ast.ASTDeclaration)
			if assert.True(t, ok, "expected ASTDeclaration for the ivar assignment") && assert.Len(t, decl.Names, 1) {
				assert.Equal(t, "@label", decl.Names[0].Name)
			}
		}
	}

	describe, ok := class.Block.Children()[2].(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected the describe method") {
		assert.Equal(t, "describe", describe.Name.Name)
		assert.Len(t, describe.Arguments, 1)
	}
}

func TestRubyParserSingletonMethodAndSuperclass(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/ruby/baseproj/types.rb")

	formatter, ok := pf.Module.Block.Children()[1].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected the Formatter class") {
		return
	}
	assert.Equal(t, "Formatter", formatter.Name.Name)

	if !assert.Len(t, formatter.Block.Children(), 2) {
		return
	}
	super, ok := formatter.Block.Children()[0].(*ast.ASTTypeExpression)
	if assert.True(t, ok, "expected the superclass as ASTTypeExpression") {
		assert.Equal(t, "Base", super.Name)
		if assert.NotNil(t, super.Namespace) {
			assert.Equal(t, "Reporting", super.Namespace.Name)
		}
	}

	build, ok := formatter.Block.Children()[1].(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected the singleton method") {
		assert.Equal(t, "build", build.Name.Name)
	}
}

func TestRubyParserLambda(t *testing.T) {
	pf := parseRbInline(t, "f = ->(v) { v.to_s }\n")
	if pf == nil {
		return
	}
	decl, ok := pf.Module.Block.Children()[0].(*ast.ASTDeclaration)
	if !assert.True(t, ok, "expected ASTDeclaration for the assignment") {
		return
	}
	if assert.Len(t, decl.Names, 1) {
		assert.Equal(t, "f", decl.Names[0].Name)
	}
	if !assert.Len(t, decl.Virtual, 1) {
		return
	}
	fn, ok := decl.Virtual[0].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected ASTFuncExpression for the lambda") {
		return
	}
	assert.Nil(t, fn.Name)
	if assert.Len(t, fn.Arguments, 1) {
		assert.Equal(t, "v", fn.Arguments[0].Identifier.Name)
	}
	if assert.Len(t, fn.Block.Children(), 1) {
		_, ok := fn.Block.Children()[0].(*ast.ASTCallExpression)
		assert.True(t, ok, "expected ASTCallExpression as the lambda body")
	}
}

func TestRubyParserRequire(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/ruby/multidep/main.rb")
	if !assert.GreaterOrEqual(t, len(pf.Module.Block.Children()), 2) {
		return
	}
	stdlib, ok := pf.Module.Block.Children()[0].(*ast.ASTImportStatement)
	if assert.True(t, ok, "expected ASTImportStatement for require") {
		assert.Equal(t, "json", stdlib.Reference.Name)
	}
	local, ok := pf.Module.Block.Children()[1].(*ast.ASTImportStatement)
	if assert.True(t, ok, "expected ASTImportStatement for require_relative") {
		assert.Equal(t, "dep1", local.Reference.Name)
	}
}

func TestRubyParserBindingFormsDeclare(t *testing.T) {
	cases := map[string]string{
		"x = compute\n":          "x",
		"MAX = 10\n":             "MAX",
		"@cache ||= build_all\n": "@cache",
		"$mode = fetch_mode\n":   "$mode",
		"a, b = pair\n":          "b",
		"first, *rest = list\n":  "rest",
	}
	for source, want := range cases {
		pf := parseRbInline(t, source)
		if pf == nil {
			continue
		}
		found := false
		for _, chld := range pf.Module.Block.Children() {
			if decl, ok := chld.(*ast.ASTDeclaration); ok {
				for _, name := range decl.Names {
					if name.Name == want {
						found = true
					}
				}
			}
		}
		assert.True(t, found, "expected a declaration binding %q in %q", want, source)
	}
}

func TestRubyParserAttachesASTParents(t *testing.T) {
	pf := parseRbSimple(t)
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
	assert.Same(t, fn, fn.Block.GetParent())
	assert.Same(t, fn.Block, call.GetParent())
	assert.Same(t, call, call.Symbol.GetParent())
}

func TestRubyParserBrokenBodyStillParses(t *testing.T) {
	// tree-sitter error recovery emits zero-width nodes for half-written
	// definitions; parsing and indexing must survive them
	for _, source := range []string{
		"def f(\n",
		"class Widget\n  def\nend\n",
		"x = \n",
		"def f\n  y.\nend\n",
		"require \n",
	} {
		if pf := parseRbInline(t, source); pf != nil {
			assert.NotNil(t, pf.Module)
		}
	}
}

func TestParserRubySimpleParse(t *testing.T) {
	checkSnapshot(t, "./fixtures/ruby/baseproj/simple.rb")
}

func TestParserRubyTypesParse(t *testing.T) {
	checkSnapshot(t, "./fixtures/ruby/baseproj/types.rb")
}

func TestParserRubyMultiDepParse(t *testing.T) {
	checkSnapshot(t, "./fixtures/ruby/multidep/main.rb")
}
