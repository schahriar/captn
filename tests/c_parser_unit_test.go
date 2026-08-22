package tests_test

import (
	"testing"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/stretchr/testify/assert"
)

func parseCSimple(t *testing.T) *cog.COGFile {
	t.Helper()
	return parseTestFile(t, "./fixtures/c/baseproj/simple.c")
}

func TestCParserModule(t *testing.T) {
	pf := parseCSimple(t)
	assert.Equal(t, "simple", pf.Module.Name)
	assert.Len(t, pf.Module.Block.Children(), 2)
}

func TestCParserImports(t *testing.T) {
	pf := parseCSimple(t)
	imp, ok := pf.Module.Block.Children()[0].(*ast.ASTImportStatement)
	if !assert.True(t, ok, "expected ASTImportStatement for the include") {
		return
	}
	if assert.NotNil(t, imp.Reference) {
		assert.Equal(t, "stdio.h", imp.Reference.Name)
	}
}

func TestCParserFunctionDefinition(t *testing.T) {
	pf := parseCSimple(t)
	fn, ok := pf.Module.Block.Children()[1].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected ASTFuncExpression for describe") {
		return
	}
	if assert.NotNil(t, fn.Name) {
		assert.Equal(t, "describe", fn.Name.Name)
	}
	if assert.Len(t, fn.Arguments, 1) {
		arg := fn.Arguments[0]
		if assert.NotNil(t, arg.Identifier) {
			assert.Equal(t, "width", arg.Identifier.Name)
		}
	}
}

func TestCParserFunctionBody(t *testing.T) {
	pf := parseCSimple(t)
	fn := pf.Module.Block.Children()[1].(*ast.ASTFuncExpression)
	if !assert.Len(t, fn.Block.Children(), 1) {
		return
	}
	ret, ok := fn.Block.Children()[0].(*ast.ASTReturnStatement)
	if !assert.True(t, ok, "expected ASTReturnStatement as the only body statement") {
		return
	}
	if !assert.Len(t, ret.Children(), 1) {
		return
	}
	call, ok := ret.Children()[0].(*ast.ASTCallExpression)
	if !assert.True(t, ok, "expected ASTCallExpression inside the return") {
		return
	}
	assert.Nil(t, call.Namespace)
	if assert.NotNil(t, call.Symbol) {
		assert.Equal(t, "printf", call.Symbol.Name)
	}
}

func TestCParserTypedefStruct(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/c/baseproj/method.c")

	widget, ok := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected the typedef struct to map to ASTFuncExpression") {
		return
	}
	if assert.NotNil(t, widget.Name) {
		assert.Equal(t, "Widget", widget.Name.Name)
	}
	if !assert.Len(t, widget.Block.Children(), 3) {
		return
	}

	// The typedef name is a second definition site and keeps its own node
	alias, ok := widget.Block.Children()[0].(*ast.ASTSymbol)
	if assert.True(t, ok, "expected the typedef name as a loose symbol") {
		assert.Equal(t, "Widget", alias.Name)
	}

	label, ok := widget.Block.Children()[1].(*ast.ASTDeclaration)
	if assert.True(t, ok, "expected ASTDeclaration for the label field") && assert.Len(t, label.Names, 1) {
		assert.Equal(t, "label", label.Names[0].Name)
	}

	area, ok := pf.Module.Block.Children()[1].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected the widget_area function") {
		return
	}
	assert.Equal(t, "widget_area", area.Name.Name)
	if assert.Len(t, area.Arguments, 1) {
		arg := area.Arguments[0]
		if assert.NotNil(t, arg.Identifier) {
			assert.Equal(t, "w", arg.Identifier.Name)
		}
		if assert.NotNil(t, arg.Type) {
			assert.Equal(t, "Widget", arg.Type.Name)
		}
	}
}

func TestCParserMacroEnumAndFunctionPointer(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/c/baseproj/types.c")
	children := pf.Module.Block.Children()
	if !assert.Len(t, children, 6) {
		return
	}

	macro, ok := children[0].(*ast.ASTDeclaration)
	if assert.True(t, ok, "expected ASTDeclaration for the macro") && assert.Len(t, macro.Names, 1) {
		assert.Equal(t, "SCALE", macro.Names[0].Name)
	}

	alias, ok := children[1].(*ast.ASTDeclaration)
	if assert.True(t, ok, "expected ASTDeclaration for typedef WidgetID") && assert.Len(t, alias.Names, 1) {
		assert.Equal(t, "WidgetID", alias.Names[0].Name)
	}

	shape, ok := children[2].(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected the enum as ASTFuncExpression") {
		assert.Equal(t, "Shape", shape.Name.Name)
		if assert.Len(t, shape.Block.Children(), 2) {
			bare, ok := shape.Block.Children()[0].(*ast.ASTSymbol)
			if assert.True(t, ok, "expected the bare enumerator as a symbol") {
				assert.Equal(t, "CIRCLE", bare.Name)
			}
			valued, ok := shape.Block.Children()[1].(*ast.ASTDeclaration)
			if assert.True(t, ok, "expected the valued enumerator as a declaration") && assert.Len(t, valued.Names, 1) {
				assert.Equal(t, "SQUARE", valued.Names[0].Name)
			}
		}
	}

	fptr, ok := children[3].(*ast.ASTDeclaration)
	if assert.True(t, ok, "expected the function-pointer typedef as a declaration, not a prototype") && assert.Len(t, fptr.Names, 1) {
		assert.Equal(t, "widget_op", fptr.Names[0].Name)
	}

	apply, ok := children[5].(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected the apply function") && assert.Len(t, apply.Arguments, 2) {
		if assert.NotNil(t, apply.Arguments[0].Type) {
			assert.Equal(t, "widget_op", apply.Arguments[0].Type.Name)
		}
		if assert.NotNil(t, apply.Arguments[1].Type) {
			assert.Equal(t, "WidgetID", apply.Arguments[1].Type.Name)
		}
	}
}

func TestCParserPrototypesInHeader(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/c/multidep/widget.h")
	children := pf.Module.Block.Children()
	if !assert.Len(t, children, 4) {
		return
	}

	guard, ok := children[0].(*ast.ASTDeclaration)
	if assert.True(t, ok, "expected the include guard as a declaration") && assert.Len(t, guard.Names, 1) {
		assert.Equal(t, "WIDGET_H", guard.Names[0].Name)
	}

	make, ok := children[2].(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected the widget_make prototype") {
		assert.Equal(t, "widget_make", make.Name.Name)
		if assert.NotNil(t, make.ReturnType) {
			assert.Equal(t, "Widget", make.ReturnType.Name)
		}
	}
}

func TestCParserForwardDeclarationKeepsName(t *testing.T) {
	pf := parseInline(t, "inline.c", "struct Point;\nstruct Point *origin(void);\n")
	if pf == nil {
		return
	}
	sym, ok := pf.Module.Block.Children()[0].(*ast.ASTSymbol)
	if assert.True(t, ok, "expected the forward declaration to leave a symbol") {
		assert.Equal(t, "Point", sym.Name)
	}
}

func TestCParserAttachesASTParents(t *testing.T) {
	pf := parseCSimple(t)
	module := pf.Module
	fn := module.Block.Children()[1].(*ast.ASTFuncExpression)
	arg := fn.Arguments[0]
	ret := fn.Block.Children()[0].(*ast.ASTReturnStatement)
	call := ret.Children()[0].(*ast.ASTCallExpression)

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

func TestCParserBrokenBodyStillParses(t *testing.T) {
	// tree-sitter error recovery emits zero-width nodes for half-written
	// definitions; parsing and indexing must survive them
	for _, source := range []string{
		"int f(\n",
		"struct Widget {\n\tint \n",
		"#define\n",
		"#include\n",
		"typedef struct {\n",
		"int x = \n",
	} {
		if pf := parseInline(t, "inline.c", source); pf != nil {
			assert.NotNil(t, pf.Module)
		}
	}
}

func TestParserCSimpleParse(t *testing.T) {
	checkSnapshot(t, "./fixtures/c/baseproj/simple.c")
}

func TestParserCTypesParse(t *testing.T) {
	checkSnapshot(t, "./fixtures/c/baseproj/types.c")
}

func TestParserCHeaderParse(t *testing.T) {
	checkSnapshot(t, "./fixtures/c/multidep/widget.h")
}

func TestParserCMultiDepParse(t *testing.T) {
	checkSnapshot(t, "./fixtures/c/multidep/main.c")
}
