package tests_test

import (
	"os"
	"testing"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/languages"
	"github.com/stretchr/testify/assert"
)

func parseCSSSimple(t *testing.T) *cog.COGFile {
	t.Helper()
	return parseTestFile(t, "./fixtures/css/baseproj/styles.css")
}

// cssFirstRule answers the first rule in the module: `:root` stays anonymous
// since a pseudo-class is not a class, so it cannot be found by name
func cssFirstRule(t *testing.T, pf *cog.COGFile) *ast.ASTFuncExpression {
	t.Helper()

	for _, chld := range pf.Module.Block.Children() {
		if fn, ok := chld.(*ast.ASTFuncExpression); ok {
			return fn
		}
	}

	assert.Fail(t, "expected a rule in the module block")
	return nil
}

func TestCSSParserModule(t *testing.T) {
	pf := parseCSSSimple(t)
	assert.Equal(t, "styles", pf.Module.Name)
	assert.Len(t, pf.Module.Block.Children(), 3)
}

func TestCSSParserRuleSetIsFunction(t *testing.T) {
	pf := parseCSSSimple(t)

	button := namedFunction(t, pf, "button")
	if button == nil {
		return
	}

	assert.True(t, cog.IsNodeOfInterest(button))
	assert.Equal(t, "button", button.Name.GetStringSource(), "the name spans the class alone, without the dot")
}

func TestCSSParserVariableDeclarationIsSymbol(t *testing.T) {
	pf := parseCSSSimple(t)

	root := cssFirstRule(t, pf)
	if root == nil {
		return
	}

	assert.Nil(t, root.Name, "a pseudo-only rule like :root stays anonymous")

	vars := []string{}
	for _, chld := range root.Block.Children() {
		if sym, ok := chld.(*ast.ASTSymbol); ok {
			vars = append(vars, sym.Name)
		}
	}

	assert.Equal(t, []string{"--accent", "--gap"}, vars)

	// A var() use resolves onto the declaration, which must be the one node there
	rng, err := pf.FindSnippetRange([]byte("--accent"))
	if !assert.NoError(t, err) {
		return
	}

	assert.Len(t, pf.FindNodesWithinRange(rng), 1)
}

func TestCSSParserVarUseIsCall(t *testing.T) {
	pf := parseCSSSimple(t)

	button := namedFunction(t, pf, "button")
	if button == nil {
		return
	}

	calls := []string{}
	eachNode(button, func(node ast.ASTNode) {
		if call, ok := node.(*ast.ASTCallExpression); ok && call.Symbol != nil {
			calls = append(calls, call.Symbol.Name)
		}
	})

	// The second use sits nested inside calc(), which is not itself a call
	assert.Equal(t, []string{"--accent", "--gap"}, calls)
}

func TestCSSParserNestedRuleAndFallback(t *testing.T) {
	pf := parseCSSSimple(t)

	// `.button .label` names itself for its first class
	chlds := pf.Module.Block.Children()
	last, ok := chlds[len(chlds)-1].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected a FuncExpression for the descendant rule, got %T", chlds[len(chlds)-1]) {
		return
	}

	if assert.NotNil(t, last.Name) {
		assert.Equal(t, "button", last.Name.Name)
	}

	calls := 0
	eachNode(last, func(node ast.ASTNode) {
		if call, ok := node.(*ast.ASTCallExpression); ok && call.Symbol != nil && call.Symbol.Name == "--accent" {
			calls++
		}
	})

	assert.Equal(t, 1, calls, "a var() with a fallback still carries its variable")
}

func TestCSSParserImport(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/css/multidep/main.css")

	imp, ok := pf.Module.Block.Children()[0].(*ast.ASTImportStatement)
	if !assert.True(t, ok, "expected an Import for @import, got %T", pf.Module.Block.Children()[0]) {
		return
	}

	if assert.NotNil(t, imp.Reference) {
		assert.Equal(t, "./theme.css", imp.Reference.Name)
	}
	assert.Equal(t, `"./theme.css"`, imp.GetStringSource())
}

func TestCSSParserMediaRuleFallsThrough(t *testing.T) {
	pf := parseInline(t, "media.css", "@media (max-width: 600px) {\n  .compact { padding: var(--gap); }\n}\n")
	if pf == nil {
		return
	}

	compact := namedFunction(t, pf, "compact")
	if compact == nil {
		return
	}

	calls := 0
	eachNode(compact, func(node ast.ASTNode) {
		if call, ok := node.(*ast.ASTCallExpression); ok && call.Symbol != nil {
			calls++
		}
	})

	assert.Equal(t, 1, calls, "rules inside @media map like any other")
}

// SCSS and LESS are degraded dialects: rules, classes and nesting map while
// preprocessor variables fall into ERROR nodes and stay unmapped bytes
func TestCSSDialectSCSSKeepsRuleStructure(t *testing.T) {
	pf := parseInline(t, "degraded.scss", "$accent: #f60;\n.button {\n  color: $accent;\n  &:hover { color: darken($accent, 10%); }\n  .label { font-weight: bold; }\n}\n")
	if pf == nil {
		return
	}

	assert.Equal(t, "scss", pf.Source.GetLanguage())

	button := namedFunction(t, pf, "button")
	if button == nil {
		return
	}

	names := []string{}
	rules := 0
	eachNode(button, func(node ast.ASTNode) {
		if fn, ok := node.(*ast.ASTFuncExpression); ok {
			rules++
			if fn.Name != nil {
				names = append(names, fn.Name.Name)
			}
		}
	})

	assert.Equal(t, 3, rules, "nested rules survive the degraded parse")
	assert.Equal(t, []string{"button", "label"}, names, "the &:hover rule stays anonymous")
}

func TestCSSDialectLESSKeepsRuleStructure(t *testing.T) {
	pf := parseInline(t, "degraded.less", "@accent: #f60;\n.button {\n  color: @accent;\n  &:hover { color: darken(@accent, 10%); }\n}\n")
	if pf == nil {
		return
	}

	assert.Equal(t, "less", pf.Source.GetLanguage())

	button := namedFunction(t, pf, "button")
	if button == nil {
		return
	}

	nested := 0
	eachNode(button, func(node ast.ASTNode) {
		if fn, ok := node.(*ast.ASTFuncExpression); ok && fn != button {
			nested++
		}
	})

	assert.Equal(t, 1, nested, "the &:hover rule maps, anonymously")
}

// A pseudo-class spells its own name with the same grammar kind a class
// uses; naming must not be hijacked by it
func TestCSSParserPseudoClassDoesNotNameRule(t *testing.T) {
	pf := parseInline(t, "pseudo.css", ".card:hover { color: red; }\ndiv:not(.aside) { color: blue; }\n")
	if pf == nil {
		return
	}

	chlds := pf.Module.Block.Children()
	if !assert.Len(t, chlds, 2) {
		return
	}

	card, ok := chlds[0].(*ast.ASTFuncExpression)
	if assert.True(t, ok) && assert.NotNil(t, card.Name) {
		assert.Equal(t, "card", card.Name.Name, "the real class outranks the pseudo-class")
	}

	not, ok := chlds[1].(*ast.ASTFuncExpression)
	if assert.True(t, ok) {
		assert.Nil(t, not.Name, "a class inside :not() excludes; it must not name the rule")
	}
}

func TestCSSParserUnquotedURLImport(t *testing.T) {
	pf := parseInline(t, "urlimp.css", "@import url(theme.css);\n")
	if pf == nil {
		return
	}

	imp, ok := pf.Module.Block.Children()[0].(*ast.ASTImportStatement)
	if assert.True(t, ok, "expected an Import for the unquoted url, got %T", pf.Module.Block.Children()[0]) && assert.NotNil(t, imp.Reference) {
		assert.Equal(t, "theme.css", imp.Reference.Name)
	}
}

func TestCSSParserVarIsCaseInsensitive(t *testing.T) {
	pf := parseInline(t, "varcase.css", ".a { color: VAR(--accent); }\n")
	if pf == nil {
		return
	}

	calls := 0
	eachNode(pf.Module, func(node ast.ASTNode) {
		if call, ok := node.(*ast.ASTCallExpression); ok && call.Symbol != nil && call.Symbol.Name == "--accent" {
			calls++
		}
	})

	assert.Equal(t, 1, calls)
}

func TestCSSClassifyImportType(t *testing.T) {
	workspace, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	cases := map[string]common.DependencyType{
		"node_modules/normalize.css/normalize.css": common.PackageDependency,
		"/x/site/node_modules/pkg/dist/style.css":  common.PackageDependency,
		"fixtures/css/multidep/theme.css":          common.LocalDependency,
		"/x/site/src/styles.css":                   common.LocalDependency,
	}

	for path, want := range cases {
		assert.Equal(t, want, languages.CSS.ClassifyImportType(common.NewSource(workspace, path, nil)), path)
	}
}

func TestCSSParserAttachesASTParents(t *testing.T) {
	pf := parseCSSSimple(t)

	root := cssFirstRule(t, pf)
	button := namedFunction(t, pf, "button")
	if root == nil || button == nil {
		return
	}

	assert.Nil(t, pf.Module.GetParent())
	assert.Same(t, pf.Module, pf.Module.Block.GetParent())
	assert.Same(t, pf.Module.Block, root.GetParent())
	assert.Same(t, button, button.Name.GetParent())

	sym, ok := root.Block.Children()[0].(*ast.ASTSymbol)
	if assert.True(t, ok, "expected the variable symbol, got %T", root.Block.Children()[0]) {
		assert.Same(t, root.Block, sym.GetParent())
	}

	call, ok := button.Block.Children()[0].(*ast.ASTCallExpression)
	if assert.True(t, ok, "expected the var() call, got %T", button.Block.Children()[0]) {
		assert.Same(t, button.Block, call.GetParent())
		assert.Same(t, call, call.Symbol.GetParent())
	}
}

func TestCSSSnapshotParse(t *testing.T) {
	checkSnapshot(t, "./fixtures/css/baseproj/styles.css")
}

func TestCSSSnapshotMultiDepParse(t *testing.T) {
	checkSnapshot(t, "./fixtures/css/multidep/main.css")
}
