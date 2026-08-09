package tests_test

import (
	"os"
	"testing"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/common"
	"github.com/stretchr/testify/assert"
)

func parsePySimple(t *testing.T) *cog.COGFile {
	t.Helper()
	return parseTestFile(t, "./fixtures/python/baseproj/simple.py")
}

func parsePyInline(t *testing.T, source string) *cog.COGFile {
	t.Helper()
	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return nil
	}
	pf, err := cog.ParseSource(t.Context(), common.NewSource(cwd, "inline.py", []byte(source)))
	if !assert.NoError(t, err, "expected %q to parse", source) {
		return nil
	}
	return pf
}

func TestPythonParserModule(t *testing.T) {
	pf := parsePySimple(t)
	assert.Equal(t, "simple", pf.Module.Name)
	assert.Len(t, pf.Module.Block.Children(), 1)
}

func TestPythonParserFunctionDefinition(t *testing.T) {
	pf := parsePySimple(t)
	fn, ok := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected ASTFuncExpression as first top-level node") {
		return
	}
	if assert.NotNil(t, fn.Name) {
		assert.Equal(t, "Symbol:x", fn.Name.String())
	}
	assert.Len(t, fn.Arguments, 1)
	if assert.NotNil(t, fn.ReturnType) {
		assert.Equal(t, "str", fn.ReturnType.Name)
	}
}

func TestPythonParserFunctionArgument(t *testing.T) {
	pf := parsePySimple(t)
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

func TestPythonParserFunctionBody(t *testing.T) {
	// Return statements are deliberately unmapped; the call inside the
	// return lands directly in the function block
	pf := parsePySimple(t)
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
		assert.Equal(t, "str", call.Symbol.Name)
	}
}

func TestPythonParserClassDefinition(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/python/baseproj/method.py")
	class, ok := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected the class to map to ASTFuncExpression") {
		return
	}
	if assert.NotNil(t, class.Name) {
		assert.Equal(t, "Widget", class.Name.Name)
	}
	if !assert.Len(t, class.Block.Children(), 1) {
		return
	}
	method, ok := class.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected the method inside the class block") {
		return
	}
	if assert.NotNil(t, method.Name) {
		assert.Equal(t, "describe", method.Name.Name)
	}
	assert.Len(t, method.Arguments, 2)
}

func TestPythonParserLambda(t *testing.T) {
	cwd, err := os.Getwd()
	assert.NoError(t, err)

	src := common.NewSource(cwd, "lambda.py", []byte("f = lambda v: str(v)\n"))
	pf, err := cog.ParseSource(t.Context(), src)
	assert.NoError(t, err)

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
	// The anonymous lambda keyword token shares the "lambda" node kind; a
	// spurious second FuncExpression would land in the block here
	if assert.Len(t, fn.Block.Children(), 1) {
		_, ok := fn.Block.Children()[0].(*ast.ASTCallExpression)
		assert.True(t, ok, "expected ASTCallExpression as the lambda body")
	}
}

func TestPythonParserAttachesASTParents(t *testing.T) {
	pf := parsePySimple(t)
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

func TestPythonParserBrokenBodyStillParses(t *testing.T) {
	// tree-sitter error recovery emits zero-width body blocks for
	// half-written definitions; parsing and indexing must survive them
	for _, source := range []string{
		"def f():\n    # todo\n\nx = 1\n",
		"def f(x):",
		"def f(x):\ndef g(y):\n    return y\n",
		"for  in xs:\n    pass\n",
		"try:\n    pass\nexcept E as :\n    pass\n",
	} {
		if pf := parsePyInline(t, source); pf != nil {
			assert.NotNil(t, pf.Module)
		}
	}
}

func TestPythonParserAttributeAssignmentDeclaresName(t *testing.T) {
	pf := parsePyInline(t, "class A:\n    def __init__(self, cb):\n        self.cb = cb\n")
	if pf == nil {
		return
	}
	class := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)
	method := class.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.Len(t, method.Block.Children(), 1) {
		return
	}
	decl, ok := method.Block.Children()[0].(*ast.ASTDeclaration)
	if !assert.True(t, ok, "expected ASTDeclaration for the attribute assignment") {
		return
	}
	if assert.Len(t, decl.Names, 1) {
		assert.Equal(t, "cb", decl.Names[0].Name)
	}
}

func TestPythonParserLoopTargetDeclaresName(t *testing.T) {
	pf := parsePyInline(t, "def run(handlers):\n    for handler in handlers:\n        handler()\n")
	if pf == nil {
		return
	}
	fn := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.Len(t, fn.Block.Children(), 2) {
		return
	}
	decl, ok := fn.Block.Children()[0].(*ast.ASTDeclaration)
	if !assert.True(t, ok, "expected ASTDeclaration for the loop target") {
		return
	}
	if assert.Len(t, decl.Names, 1) {
		assert.Equal(t, "handler", decl.Names[0].Name)
	}
	_, ok = fn.Block.Children()[1].(*ast.ASTCallExpression)
	assert.True(t, ok, "expected the loop body call")
}

func TestPythonParserBindingFormsDeclare(t *testing.T) {
	cases := map[string]string{
		"def go(g):\n    if (n := g()):\n        n()\n":               "n",
		"def go():\n    with open(\"x\") as fh:\n        fh.read()\n": "fh",
		"def go(xs):\n    try:\n        xs()\n    except ValueError as err:\n        err.args\n": "err",
	}
	for source, want := range cases {
		pf := parsePyInline(t, source)
		if pf == nil {
			continue
		}
		fn, ok := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)
		if !assert.True(t, ok) {
			continue
		}
		found := false
		for _, chld := range fn.Block.Children() {
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

func TestPythonParserUnpackingDeclaresAllNames(t *testing.T) {
	pf := parsePyInline(t, "first, *rest = xs()\n")
	if pf == nil {
		return
	}
	decl, ok := pf.Module.Block.Children()[0].(*ast.ASTDeclaration)
	if !assert.True(t, ok, "expected ASTDeclaration for the unpacking assignment") {
		return
	}
	names := common.Map(decl.Names, func(s *ast.ASTSymbol) string { return s.Name })
	assert.Equal(t, []string{"first", "rest"}, names)
}

func TestPythonParserComprehensionTargetDeclaresName(t *testing.T) {
	pf := parsePyInline(t, "def use(fs):\n    return [f() for f in fs]\n")
	if pf == nil {
		return
	}
	fn, ok := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.True(t, ok) {
		return
	}
	found := false
	for _, chld := range fn.Block.Children() {
		if decl, ok := chld.(*ast.ASTDeclaration); ok {
			for _, name := range decl.Names {
				if name.Name == "f" {
					found = true
				}
			}
		}
	}
	assert.True(t, found, "expected a declaration binding the comprehension variable")
}

func TestPythonParserFutureImport(t *testing.T) {
	pf := parsePyInline(t, "from __future__ import annotations\n")
	if pf == nil {
		return
	}
	if !assert.Len(t, pf.Module.Block.Children(), 1) {
		return
	}
	imp, ok := pf.Module.Block.Children()[0].(*ast.ASTImportStatement)
	if assert.True(t, ok, "expected ASTImportStatement for the future import") {
		assert.Equal(t, "annotations", imp.Reference.Name)
	}
}

func TestPythonParserRelativeWildcardImport(t *testing.T) {
	pf := parsePyInline(t, "from .helpers import *\n")
	if pf == nil {
		return
	}
	if !assert.Len(t, pf.Module.Block.Children(), 1) {
		return
	}
	imp, ok := pf.Module.Block.Children()[0].(*ast.ASTImportStatement)
	if assert.True(t, ok, "expected ASTImportStatement for the wildcard import") {
		assert.Equal(t, "helpers", imp.Reference.Name)
	}
}

func TestParserPythonSimpleParse(t *testing.T) {
	checkSnapshot(t, "./fixtures/python/baseproj/simple.py")
}

func TestParserPythonMultiDepParse(t *testing.T) {
	checkSnapshot(t, "./fixtures/python/multidep/main.py")
}
