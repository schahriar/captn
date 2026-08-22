package tests_test

import (
	"testing"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/stretchr/testify/assert"
)

func TestTypeExpressionC(t *testing.T) {
	pf := parseInline(t, "types.c",
		"struct Shape;\ntypedef int WidgetID;\nWidgetID f(struct Shape *s, unsigned long n, WidgetID id);\n")
	if pf == nil {
		return
	}

	fn := firstFunction(t, pf)
	if fn == nil {
		return
	}

	if !assert.Len(t, fn.Arguments, 3) {
		return
	}

	assertTypeSource(t, fn.Arguments[0].Type, "Shape")
	assert.Nil(t, fn.Arguments[1].Type, "a sized type is a keyword with no definition site")
	assertTypeSource(t, fn.Arguments[2].Type, "WidgetID")
	assertTypeSource(t, fn.ReturnType, "WidgetID")
}

func TestTypeExpressionCPPQualifiedAndTemplate(t *testing.T) {
	pf := parseInline(t, "types.cpp",
		"#include <map>\nvoid f(std::map<std::string, Widget> lookup);\n")
	if pf == nil {
		return
	}

	fn := firstFunction(t, pf)
	if fn == nil {
		return
	}

	if !assert.Len(t, fn.Arguments, 1) {
		return
	}

	lookup := fn.Arguments[0].Type
	if !assert.NotNil(t, lookup) {
		return
	}

	assertTypeSource(t, lookup, "map")
	if assert.NotNil(t, lookup.Namespace) {
		assert.Equal(t, "std", lookup.Namespace.Name)
	}
	if assert.Len(t, lookup.Arguments, 2) {
		assertTypeSource(t, lookup.Arguments[0], "string")
		assertTypeSource(t, lookup.Arguments[1], "Widget")
	}
}

func TestTypeExpressionCPPTrailingReturn(t *testing.T) {
	pf := parseInline(t, "types.cpp",
		"auto make() -> gadgets::Widget { return {}; }\n")
	if pf == nil {
		return
	}

	fn := firstFunction(t, pf)
	if fn == nil {
		return
	}

	assertTypeSource(t, fn.ReturnType, "Widget")
	if assert.NotNil(t, fn.ReturnType) && assert.NotNil(t, fn.ReturnType.Namespace) {
		assert.Equal(t, "gadgets", fn.ReturnType.Namespace.Name)
	}
}

func TestTypeExpressionCPPPointerFoldsToHead(t *testing.T) {
	// Pointers and arrays live in the declarator, so the annotation stays on
	// the named head alone
	pf := parseInline(t, "types.cpp",
		"Widget *make(const Widget **grid);\n")
	if pf == nil {
		return
	}

	fn := firstFunction(t, pf)
	if fn == nil {
		return
	}

	assertTypeSource(t, fn.ReturnType, "Widget")
	if assert.Len(t, fn.Arguments, 1) {
		assertTypeSource(t, fn.Arguments[0].Type, "Widget")
	}
}

func TestCTypeShapesEveryTypeSpansItsIdentifier(t *testing.T) {
	for _, path := range []string{
		"./fixtures/c/baseproj/method.c",
		"./fixtures/c/baseproj/types.c",
		"./fixtures/c/multidep/widget.h",
		"./fixtures/cpp/baseproj/method.cpp",
		"./fixtures/cpp/baseproj/types.cpp",
		"./fixtures/cpp/multidep/gadget.hpp",
	} {
		pf := parseTestFile(t, path)
		if pf == nil {
			continue
		}

		var types []*ast.ASTTypeExpression
		cCollectTypes(pf.Module, &types)

		for _, texpr := range types {
			assert.Equal(t, texpr.Name, texpr.GetStringSource(),
				"%v: type expression must span the identifier alone", path)
		}
	}
}
