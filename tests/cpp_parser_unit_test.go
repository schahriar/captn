package tests_test

import (
	"testing"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/stretchr/testify/assert"
)

func TestCPPParserNamespaceAndClass(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/cpp/baseproj/method.cpp")

	ns, ok := pf.Module.Block.Children()[1].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected the namespace as ASTFuncExpression") {
		return
	}
	assert.Equal(t, "gadgets", ns.Name.Name)
	if !assert.Len(t, ns.Block.Children(), 3) {
		return
	}

	class, ok := ns.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected the class as ASTFuncExpression") {
		return
	}
	assert.Equal(t, "Widget", class.Name.Name)
	if !assert.Len(t, class.Block.Children(), 5) {
		return
	}

	ctor, ok := class.Block.Children()[0].(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected the constructor prototype") {
		assert.Equal(t, "Widget", ctor.Name.Name)
		if assert.Len(t, ctor.Arguments, 1) {
			assert.Equal(t, "width", ctor.Arguments[0].Identifier.Name)
		}
	}

	area, ok := class.Block.Children()[1].(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected the area prototype") {
		assert.Equal(t, "area", area.Name.Name)
	}

	describe, ok := class.Block.Children()[2].(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected the inline describe method") {
		assert.Equal(t, "describe", describe.Name.Name)
		if assert.NotNil(t, describe.ReturnType) {
			assert.Equal(t, "string", describe.ReturnType.Name)
			if assert.NotNil(t, describe.ReturnType.Namespace) {
				assert.Equal(t, "std", describe.ReturnType.Namespace.Name)
			}
		}
	}

	label, ok := class.Block.Children()[3].(*ast.ASTDeclaration)
	if assert.True(t, ok, "expected the label_ member") && assert.Len(t, label.Names, 1) {
		assert.Equal(t, "label_", label.Names[0].Name)
		if assert.NotNil(t, label.Type) {
			assert.Equal(t, "string", label.Type.Name)
		}
	}

	outCtor, ok := ns.Block.Children()[1].(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected the out-of-line constructor") {
		assert.Equal(t, "Widget", outCtor.Name.Name)
	}

	outArea, ok := ns.Block.Children()[2].(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected the out-of-line area definition") {
		assert.Equal(t, "area", outArea.Name.Name)
	}
}

func TestCPPParserInheritanceAndAlias(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/cpp/baseproj/types.cpp")
	ns := pf.Module.Block.Children()[1].(*ast.ASTFuncExpression)
	if !assert.Len(t, ns.Block.Children(), 5) {
		return
	}

	base, ok := ns.Block.Children()[0].(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected struct Base") {
		assert.Equal(t, "Base", base.Name.Name)
	}

	card, ok := ns.Block.Children()[1].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected class Card") {
		return
	}

	heritage, ok := card.Block.Children()[0].(*ast.ASTTypeExpression)
	if assert.True(t, ok, "expected the base class as ASTTypeExpression") {
		assert.Equal(t, "Base", heritage.Name)
	}

	alias, ok := ns.Block.Children()[2].(*ast.ASTDeclaration)
	if assert.True(t, ok, "expected the using alias as ASTDeclaration") {
		if assert.Len(t, alias.Names, 1) {
			assert.Equal(t, "CardList", alias.Names[0].Name)
		}
		if assert.NotNil(t, alias.Type) {
			assert.Equal(t, "vector", alias.Type.Name)
			if assert.NotNil(t, alias.Type.Namespace) {
				assert.Equal(t, "std", alias.Type.Namespace.Name)
			}
			if assert.Len(t, alias.Type.Arguments, 1) {
				assert.Equal(t, "Card", alias.Type.Arguments[0].Name)
			}
		}
	}

	shape, ok := ns.Block.Children()[3].(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected enum class Shape") {
		assert.Equal(t, "Shape", shape.Name.Name)
	}
}

func TestCPPParserTemplateFunction(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/cpp/baseproj/types.cpp")
	ns := pf.Module.Block.Children()[1].(*ast.ASTFuncExpression)

	last, ok := ns.Block.Children()[4].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected the template function") {
		return
	}
	assert.Equal(t, "last", last.Name.Name)

	if !assert.Len(t, last.Arguments, 3) {
		return
	}

	// The type parameter is a definition site of its own
	tparam := last.Arguments[0]
	if assert.NotNil(t, tparam.Identifier) {
		assert.Equal(t, "T", tparam.Identifier.Name)
	}

	cards := last.Arguments[1]
	if assert.NotNil(t, cards.Type) {
		assert.Equal(t, "CardList", cards.Type.Name)
	}

	fallback := last.Arguments[2]
	if assert.NotNil(t, fallback.Type) {
		assert.Equal(t, "T", fallback.Type.Name)
	}

	if assert.NotNil(t, last.ReturnType) {
		assert.Equal(t, "T", last.ReturnType.Name)
	}

	scale, ok := last.Block.Children()[0].(*ast.ASTDeclaration)
	if !assert.True(t, ok, "expected the lambda declaration") {
		return
	}
	if assert.Len(t, scale.Names, 1) {
		assert.Equal(t, "scale", scale.Names[0].Name)
	}
	if assert.Len(t, scale.Virtual, 1) {
		lambda, ok := scale.Virtual[0].(*ast.ASTFuncExpression)
		if assert.True(t, ok, "expected ASTFuncExpression for the lambda") {
			assert.Nil(t, lambda.Name)
			if assert.Len(t, lambda.Arguments, 1) {
				assert.Equal(t, "v", lambda.Arguments[0].Identifier.Name)
			}
		}
	}
}

func TestCPPParserCallsAndCasts(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/cpp/multidep/main.cpp")

	run, ok := pf.Module.Block.Children()[2].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected the run function") {
		return
	}
	if !assert.Len(t, run.Block.Children(), 3) {
		return
	}

	g, ok := run.Block.Children()[0].(*ast.ASTDeclaration)
	if assert.True(t, ok, "expected the gadget declaration") {
		if assert.Len(t, g.Names, 1) {
			assert.Equal(t, "g", g.Names[0].Name)
		}
		if assert.NotNil(t, g.Type) {
			assert.Equal(t, "Gadget", g.Type.Name)
			if assert.NotNil(t, g.Type.Namespace) {
				assert.Equal(t, "gadgets", g.Type.Namespace.Name)
			}
		}
	}

	label, ok := run.Block.Children()[1].(*ast.ASTDeclaration)
	if assert.True(t, ok, "expected the label declaration") && assert.Len(t, label.Virtual, 1) {
		call, ok := label.Virtual[0].(*ast.ASTCallExpression)
		if assert.True(t, ok, "expected the describe call") {
			assert.Equal(t, "describe", call.Symbol.Name)
			if assert.NotNil(t, call.Namespace) {
				assert.Equal(t, "g", call.Namespace.Name)
			}
		}
	}

	ret, ok := run.Block.Children()[2].(*ast.ASTReturnStatement)
	if !assert.True(t, ok, "expected the return statement") {
		return
	}

	// static_cast is a keyword, not a callee; the calls inside still map
	var names []string
	for _, chld := range ret.Children() {
		if call, ok := chld.(*ast.ASTCallExpression); ok && call.Symbol != nil {
			names = append(names, call.Symbol.Name)
		}
	}
	assert.Equal(t, []string{"size", "rank"}, names)
}

func TestCPPParserBrokenBodyStillParses(t *testing.T) {
	for _, source := range []string{
		"class Widget {\n\tvoid \n",
		"template <typename \n",
		"namespace gadgets {\n",
		"auto f = [](\n",
		"using X = \n",
	} {
		if pf := parseInline(t, "inline.cpp", source); pf != nil {
			assert.NotNil(t, pf.Module)
		}
	}
}

func TestParserCPPMethodParse(t *testing.T) {
	checkSnapshot(t, "./fixtures/cpp/baseproj/method.cpp")
}

func TestParserCPPTypesParse(t *testing.T) {
	checkSnapshot(t, "./fixtures/cpp/baseproj/types.cpp")
}

func TestParserCPPMultiDepParse(t *testing.T) {
	checkSnapshot(t, "./fixtures/cpp/multidep/main.cpp")
}
