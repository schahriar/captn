package tests_test

import (
	"strings"
	"testing"

	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/common"
	"github.com/stretchr/testify/assert"
)

// Zero-width range at EOF: pointRange widens past the buffer end
func TestPointRangeAtEOF(t *testing.T) {
	pf := parseSwiftSimple(t)
	last := len(pf.Source.Buffer)
	pos := common.NewFilePosition(pf.Source, 0, 0, last)
	r := common.NewFileRange(pf.Source, pos, pos)

	assert.NotPanics(t, func() {
		pf.FindNodesWithinRange(r)
		pf.FindTightestEnclosingNode(r, cog.IsNodeOfInterest)
	})
}

// FindSnippetRange returns a zero-width range for an empty snippet, which now
// takes the point branch
func TestEmptySnippetRange(t *testing.T) {
	pf := parseSwiftSimple(t)
	r, err := pf.FindSnippetRange([]byte(""))
	if !assert.NoError(t, err) {
		return
	}
	assert.NotPanics(t, func() { pf.FindNodesWithinRange(r) })
}

// Modern Swift the fixtures do not cover must not panic or fail to parse
func TestSwiftModernSyntaxParses(t *testing.T) {
	sources := map[string]string{
		"generics":          "func f<T: Equatable>(a: T, b: T) -> Bool { return a == b }\n",
		"async await":       "func load() async throws -> Data {\n    return try await fetch()\n}\n",
		"property wrapper":  "struct V {\n    @State private var count: Int = 0\n}\n",
		"result builder":    "var body: some View {\n    VStack {\n        Text(\"hi\")\n    }\n}\n",
		"macro":             "@attached(member)\npublic macro Observable() = #externalMacro(module: \"M\", type: \"O\")\n",
		"actor":             "actor Counter {\n    var n = 0\n    func bump() { n += 1 }\n}\n",
		"trailing closure":  "func go() {\n    items.map { $0.value }.forEach { use($0) }\n}\n",
		"guard let chain":   "func go() {\n    guard let a = x, let b = y else { return }\n    use(a, b)\n}\n",
		"nested types":      "struct Outer {\n    struct Inner {\n        func deep() -> Int { return 1 }\n    }\n}\n",
		"opaque + optional": "func make() -> (any Shape)? { return nil }\n",
		"subscript":         "extension A {\n    subscript(i: Int) -> Int { return i }\n}\n",
		"unicode":           "func 日本語(値: Int) -> String {\n    return \"→ \\(値) ✓\"\n}\n",
		"crlf":              "func f() -> Int {\r\n    return g()\r\n}\r\n",
		"only comments":     "// nothing here\n/* block */\n",
	}

	for name, src := range sources {
		t.Run(name, func(t *testing.T) {
			cwd := t.TempDir()
			assert.NotPanics(t, func() {
				pf, err := cog.ParseSource(t.Context(), common.NewSource(cwd, "edge.swift", []byte(src)))
				if assert.NoError(t, err, "expected %q to parse", name) {
					assert.NotNil(t, pf.Module)
					// indexing must not collide or blow up
					assert.NotPanics(t, func() { pf.IndexNodes() })
				}
			})
		})
	}
	_ = strings.TrimSpace
}
