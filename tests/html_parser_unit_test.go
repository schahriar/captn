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

func parseHTMLSimple(t *testing.T) *cog.COGFile {
	t.Helper()
	return parseTestFile(t, "./fixtures/html/baseproj/index.html")
}

func htmlChildElements(fn *ast.ASTFuncExpression) []*ast.ASTFuncExpression {
	elems := []*ast.ASTFuncExpression{}
	for _, chld := range fn.Block.Children() {
		if elem, ok := chld.(*ast.ASTFuncExpression); ok {
			elems = append(elems, elem)
		}
	}
	return elems
}

func htmlElementNamed(t *testing.T, pf *cog.COGFile, name string) *ast.ASTFuncExpression {
	t.Helper()

	var found *ast.ASTFuncExpression

	eachNode(pf.Module, func(node ast.ASTNode) {
		if fn, ok := node.(*ast.ASTFuncExpression); ok && found == nil && fn.Name != nil && fn.Name.Name == name {
			found = fn
		}
	})

	if found == nil {
		assert.Failf(t, "element not found", "expected an element named %q", name)
	}

	return found
}

func TestHTMLParserModule(t *testing.T) {
	pf := parseHTMLSimple(t)
	assert.Equal(t, "index", pf.Module.Name)
}

func TestHTMLParserElementsAreFunctions(t *testing.T) {
	pf := parseHTMLSimple(t)

	html := namedFunction(t, pf, "html")
	if html == nil {
		return
	}

	assert.True(t, cog.IsNodeOfInterest(html))

	nested := htmlChildElements(html)
	if !assert.Len(t, nested, 2) {
		return
	}

	if assert.NotNil(t, nested[0].Name) {
		assert.Equal(t, "head", nested[0].Name.Name)
	}
	if assert.NotNil(t, nested[1].Name) {
		assert.Equal(t, "body", nested[1].Name.Name)
	}

	head := nested[0]
	inHead := htmlChildElements(head)
	if !assert.Len(t, inHead, 2) {
		return
	}

	if assert.NotNil(t, inHead[0].Name) {
		assert.Equal(t, "link", inHead[0].Name.Name)
	}
	if assert.NotNil(t, inHead[1].Name) {
		assert.Equal(t, "script", inHead[1].Name.Name)
	}
}

func TestHTMLParserAttributesAreArguments(t *testing.T) {
	pf := parseHTMLSimple(t)

	html := namedFunction(t, pf, "html")
	if html == nil || !assert.Len(t, html.Arguments, 1) {
		return
	}

	if assert.NotNil(t, html.Arguments[0].Identifier) {
		assert.Equal(t, "lang", html.Arguments[0].Identifier.Name)
	}
	assert.Equal(t, `lang="en"`, html.Arguments[0].GetStringSource())

	div := htmlElementNamed(t, pf, "div")
	if div == nil || !assert.Len(t, div.Arguments, 2) {
		return
	}

	if assert.NotNil(t, div.Arguments[0].Identifier) {
		assert.Equal(t, "class", div.Arguments[0].Identifier.Name)
	}
	if assert.NotNil(t, div.Arguments[1].Identifier) {
		assert.Equal(t, "id", div.Arguments[1].Identifier.Name)
	}

	// A value-less attribute is its own name; the symbol wins that range
	script := htmlElementNamed(t, pf, "script")
	if script == nil || !assert.Len(t, script.Arguments, 2) {
		return
	}

	if assert.NotNil(t, script.Arguments[1].Identifier) {
		assert.Equal(t, "defer", script.Arguments[1].Identifier.Name)
	}
}

func TestHTMLParserSelfClosingElement(t *testing.T) {
	pf := parseHTMLSimple(t)

	br := htmlElementNamed(t, pf, "div")
	if br == nil {
		return
	}

	found := false
	eachNode(br, func(node ast.ASTNode) {
		if fn, ok := node.(*ast.ASTFuncExpression); ok && fn.Name != nil && fn.Name.Name == "br" {
			found = true
			assert.Empty(t, fn.Arguments)
		}
	})

	assert.True(t, found, "expected the self-closing element inside the div")

	// A self-closing element IS its opening tag, so its block keeps the
	// shadow and a snippet on it roots at the enclosing element
	rng, err := pf.FindSnippetRange([]byte("<br/>"))
	if !assert.NoError(t, err) {
		return
	}

	root := pf.FindTightestEnclosingNode(rng, cog.IsNodeOfInterest)
	fn, ok := root.(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected an element, got %T", root) && assert.NotNil(t, fn.Name) {
		assert.Equal(t, "div", fn.Name.Name)
	}
}

func TestHTMLParserAttachesASTParents(t *testing.T) {
	pf := parseHTMLSimple(t)

	html := namedFunction(t, pf, "html")
	if html == nil {
		return
	}

	assert.Same(t, pf.Module.Block, html.GetParent())
	assert.Same(t, html, html.Name.GetParent())
	assert.Same(t, html, html.Arguments[0].GetParent())

	body := htmlChildElements(html)[1]
	assert.Same(t, html, body.GetParent().(*ast.ASTBlock).GetParent())
}

// A snippet inside an element roots at that element: the block narrows onto
// the opening tag so the element itself stays visible to range queries
func TestHTMLParserSnippetRootsAtElement(t *testing.T) {
	pf := parseHTMLSimple(t)

	rng, err := pf.FindSnippetRange([]byte("Click"))
	if !assert.NoError(t, err) {
		return
	}

	root := pf.FindTightestEnclosingNode(rng, cog.IsNodeOfInterest)
	fn, ok := root.(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected an element, got %T", root) && assert.NotNil(t, fn.Name) {
		assert.Equal(t, "div", fn.Name.Name)
	}
}

func TestHTMLParserBrokenMarkupStillParses(t *testing.T) {
	pf := parseInline(t, "broken.html", "<div><span>text</div>\n")
	if pf == nil {
		return
	}

	div := htmlElementNamed(t, pf, "div")
	assert.NotNil(t, div, "error recovery keeps the outer element")
}

func TestHTMLClassifyImportType(t *testing.T) {
	workspace, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	cases := map[string]common.DependencyType{
		"node_modules/lit/index.html":       common.PackageDependency,
		"fixtures/html/baseproj/index.html": common.LocalDependency,
	}

	for path, want := range cases {
		assert.Equal(t, want, languages.HTML.ClassifyImportType(common.NewSource(workspace, path, nil)), path)
	}
}

func TestHTMLSnapshotParse(t *testing.T) {
	checkSnapshot(t, "./fixtures/html/baseproj/index.html")
}
