package tests_test

import (
	"os"
	"strings"
	"testing"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/languages"
	"github.com/stretchr/testify/assert"
)

// typeShape renders a type expression as head{arg, arg} so a whole fold can be
// compared in one assertion.
func typeShape(texpr *ast.ASTTypeExpression) string {
	if texpr == nil {
		return "<nil>"
	}

	name := texpr.Name
	if texpr.Namespace != nil {
		name = texpr.Namespace.Name + "." + name
	}

	if len(texpr.Arguments) == 0 {
		return name
	}

	args := common.Map(texpr.Arguments, typeShape)

	return name + "{" + strings.Join(args, ", ") + "}"
}

// A generic head owns what it subscripts; a headless composite (union, tuple,
// the list inside Callable) contributes its members flat under the first name.
func TestPythonTypeExpressionFoldsFlat(t *testing.T) {
	pf := parseInline(t, "flat.py", "def f(a: dict[str, list[Widget]], b: A | B, c: Callable[[int], str], d: typing.List[int], e: list[int] | dict[str, int], f: tuple[int, str] | None) -> None:\n    return None\n")
	if pf == nil {
		return
	}

	fn := firstFunction(t, pf)
	if !assert.Len(t, fn.Arguments, 6) {
		return
	}

	shapes := common.Map(fn.Arguments, func(arg *ast.ASTFuncArgument) string { return typeShape(arg.Type) })

	assert.Equal(t, []string{
		"dict{str, list{Widget}}",
		"A{B}",
		"Callable{int, str}",
		"typing.List{int}",
		"list{int, dict{str, int}}",
		"tuple{int, str}",
	}, shapes)

	// Every named type sits on its own identifier so a definition resolves
	for _, arg := range fn.Arguments {
		assertTypeSource(t, arg.Type, arg.Type.Name)
		for _, nested := range arg.Type.Arguments {
			assertTypeSource(t, nested, nested.Name)
		}
	}

	assert.Nil(t, fn.ReturnType, "-> None names no identifier")
}

func TestPythonTypeParametersAreArguments(t *testing.T) {
	pf := parseInline(t, "params.py", "class C[T]:\n    def f(self, a: T) -> T:\n        return a\n\ndef g[T: int, *Ts, **P](a: T) -> T:\n    return a\n")
	if pf == nil {
		return
	}

	class, ok := pf.Module.Block.Children()[0].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected the class to map to ASTFuncExpression") {
		return
	}

	if assert.Len(t, class.Arguments, 1) {
		assert.Equal(t, "T", class.Arguments[0].Identifier.Name)
		assert.Equal(t, "T", class.Arguments[0].Identifier.GetStringSource())
		assert.Nil(t, class.Arguments[0].Type)
	}

	// Type parameters are arguments, never declarations in the block
	assert.Len(t, class.Block.Children(), 1)

	fn, ok := pf.Module.Block.Children()[1].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected the generic function") {
		return
	}

	if !assert.Len(t, fn.Arguments, 4, "type parameters precede value parameters") {
		return
	}

	names := common.Map(fn.Arguments, func(arg *ast.ASTFuncArgument) string { return arg.Identifier.Name })
	assert.Equal(t, []string{"T", "Ts", "P", "a"}, names)

	containers := common.Map(fn.Arguments, func(arg *ast.ASTFuncArgument) string { return arg.GetStringSource() })
	assert.Equal(t, []string{"T: int", "*Ts", "**P", "a: T"}, containers)

	assertTypeSource(t, fn.Arguments[0].Type, "int")
	assert.Nil(t, fn.Arguments[1].Type)
	assert.Nil(t, fn.Arguments[2].Type)
	assertTypeSource(t, fn.Arguments[3].Type, "T")
	assertTypeSource(t, fn.ReturnType, "T")
}

func TestPythonTypeAliasDeclares(t *testing.T) {
	pf := parseInline(t, "alias.py", "type X = int\ntype Table[T] = dict[str, T]\n")
	if pf == nil {
		return
	}

	if !assert.Len(t, pf.Module.Block.Children(), 2) {
		return
	}

	alias, ok := pf.Module.Block.Children()[0].(*ast.ASTDeclaration)
	if !assert.True(t, ok, "expected ASTDeclaration for the type alias") {
		return
	}

	if assert.Len(t, alias.Names, 1) {
		assert.Equal(t, "X", alias.Names[0].Name)
		assert.Equal(t, "X", alias.Names[0].GetStringSource())
	}
	assertTypeSource(t, alias.Type, "int")

	// A generic alias declares its parameters too: a use of T on the right
	// resolves back onto the left
	generic, ok := pf.Module.Block.Children()[1].(*ast.ASTDeclaration)
	if !assert.True(t, ok, "expected ASTDeclaration for the generic alias") {
		return
	}

	names := common.Map(generic.Names, func(s *ast.ASTSymbol) string { return s.Name })
	assert.Equal(t, []string{"Table", "T"}, names)
	assert.Equal(t, "dict{str, T}", typeShape(generic.Type))
}

// typeshed declares the typing special forms as bare annotated assignments
// (`Optional: _SpecialForm`); the name is what a use of Optional resolves to.
func TestPythonBareAnnotatedAssignmentDeclaresType(t *testing.T) {
	pf := parseInline(t, "forms.pyi", "Optional: _SpecialForm\n")
	if pf == nil {
		return
	}

	decl, ok := pf.Module.Block.Children()[0].(*ast.ASTDeclaration)
	if !assert.True(t, ok, "expected ASTDeclaration for the annotated assignment") {
		return
	}

	if assert.Len(t, decl.Names, 1) {
		assert.Equal(t, "Optional", decl.Names[0].Name)
	}
	assertTypeSource(t, decl.Type, "_SpecialForm")
}

// pyright answers with the whole declaring node for special forms, annotated
// parameters and type parameters; the reply is cut back to its first name.
func TestPythonNormalizeDefinitionRange(t *testing.T) {
	cases := []struct {
		name   string
		source string
		reply  string
		want   string
	}{
		{"special form", "Optional: _SpecialForm\n", "Optional: _SpecialForm", "Optional"},
		{"annotated parameter", "def f(fn: Callable[[int], str]) -> str: ...\n", "fn: Callable[[int], str]", "fn"},
		{"type parameter list", "class Pair[K, V]: ...\n", "[K, V]", "K"},
		{"bounded type parameter", "def g[T: int](a: T) -> T: ...\n", "[T: int]", "T"},
		{"multi-line type parameter list", "class C[\n    K,\n    V,\n]: ...\n", "[\n    K,\n    V,\n]", "K"},
		{"unicode identifier", "café: int\n", "café: int", "café"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := common.NewSource(t.TempDir(), "n.py", []byte(tc.source))

			at := strings.Index(tc.source, tc.reply)
			if !assert.NotEqual(t, -1, at, "reply should exist in fixture") {
				return
			}

			reply := spanAt(src, at, at+len(tc.reply))
			got := languages.Python.NormalizeDefinitionRange(src, reply)

			if !assert.NotNil(t, got) {
				return
			}

			assert.Equal(t, tc.want, string(got.GetBytes()))

			// Line and column must follow the byte offsets or the interval
			// index looks in the wrong place
			wantStart := strings.Index(tc.source, tc.want)
			expected := spanAt(src, wantStart, wantStart+len(tc.want))
			assert.Equal(t, expected.Start, got.Start)
			assert.Equal(t, expected.End, got.End)
		})
	}
}

// Replies that already are one name, hold no name at all, or span nothing (an
// import resolves to 0:0 of its module) pass through untouched.
func TestPythonNormalizeDefinitionRangeLeavesOthersAlone(t *testing.T) {
	src := common.NewSource(t.TempDir(), "n.py", []byte("class Widget: ...\n"))

	for _, span := range []*common.FileRange{
		spanAt(src, 6, 12),  // Widget
		spanAt(src, 0, 0),   // empty
		spanAt(src, 14, 17), // ...
	} {
		assert.Same(t, span, languages.Python.NormalizeDefinitionRange(src, span))
	}
}

// spanAt builds a range over buf[from:to] with line and column derived from
// the buffer, the way a language server reply arrives.
func spanAt(src *common.Source, from int, to int) *common.FileRange {
	pos := func(at int) common.FilePosition {
		line, col := 0, 0
		for i := 0; i < at; i++ {
			if src.Buffer[i] == '\n' {
				line++
				col = 0
			} else {
				col++
			}
		}
		return common.NewFilePosition(src, line, col, at)
	}

	return common.NewFileRange(src, pos(from), pos(to))
}

// Every typing special form used in a signature resolves into typeshed, where
// pyright's reply is the whole `Optional: _SpecialForm` statement; without
// narrowing the search fails with "expected 1 node ... received 3".
func TestPythonSearchSnippetResolvesTypingForms(t *testing.T) {
	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	wspace := cog.NewWorkspace(cwd)

	for snippet, want := range map[string]string{
		"def maybe(a: Optional[int]) -> None:":                            "maybe",
		"def apply(fn: Callable[[int], str], v: Union[int, str]) -> str:": "apply",
		"def tagged(a: Annotated[int, \"x\"], b: List[int]) -> None:":     "tagged",
		"return fn(1)": "apply",
	} {
		_, root, err := wspace.SearchSnippet(t.Context(), "./fixtures/python/baseproj/typing_forms.py", snippet)
		if !assert.NoError(t, err, snippet) {
			continue
		}

		fn, ok := root.(*ast.ASTFuncExpression)
		if assert.True(t, ok, "expected root to be the enclosing function for %q, got %T", snippet, root) {
			assert.Equal(t, want, fn.Name.Name, snippet)
		}
	}
}

// A type parameter and a type alias are definition sites inside the searched
// file itself; a use of T or Count must find exactly one node there.
func TestPythonSearchSnippetResolvesTypeParametersAndAliases(t *testing.T) {
	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	wspace := cog.NewWorkspace(cwd)

	for snippet, want := range map[string]string{
		"def get(self, a: T) -> T:":          "get",
		"def second(self, k: K, v: V) -> V:": "second",
		"def bounded[T: int](a: T) -> T:":    "bounded",
		"def count(a: Count) -> Count:":      "count",
	} {
		og, root, err := wspace.SearchSnippet(t.Context(), "./fixtures/python/baseproj/typing_forms.py", snippet)
		if !assert.NoError(t, err, snippet) {
			continue
		}

		fn, ok := root.(*ast.ASTFuncExpression)
		if !assert.True(t, ok, "expected root to be the enclosing function for %q, got %T", snippet, root) {
			continue
		}

		assert.Equal(t, want, fn.Name.Name, snippet)

		// T resolves onto the class's own parameter, so the class is a vertex
		// of the method's graph
		if want == "get" {
			adj, err := og.Graph.AdjacencyMap()
			if !assert.NoError(t, err) {
				continue
			}

			class, ok := fn.GetParent().Nearest(cog.IsNodeOfInterest).(*ast.ASTFuncExpression)
			if assert.True(t, ok, "expected the enclosing class") {
				assert.Equal(t, "Box", class.Name.Name)
				assert.Contains(t, adj, ast.GetHash(class), "expected the declaring class in the graph")
			}
		}
	}
}
