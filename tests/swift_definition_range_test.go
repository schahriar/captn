package tests_test

import (
	"strings"
	"testing"

	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/languages"
	"github.com/stretchr/testify/assert"
)

// pointAt builds the reply shape sourcekit-lsp actually sends for
// textDocument/definition: a zero-width position at the start of the name.
func pointAt(src *common.Source, at int) *common.FileRange {
	line, col := 0, at
	for i := 0; i < at; i++ {
		if src.Buffer[i] == '\n' {
			line++
			col = at - i - 1
		}
	}
	pos := common.NewFilePosition(src, line, col, at)
	return common.NewFileRange(src, pos, pos)
}

func TestSwiftNormalizeDefinitionRange(t *testing.T) {
	cases := []struct {
		name   string
		source string
		anchor string
		want   string
	}{
		{"function name", "public func getExampleText() -> String {}\n", "getExampleText", "getExampleText"},
		{"type name", "struct Container {}\n", "Container", "Container"},
		{"leading underscore", "func _private() {}\n", "_private", "_private"},
		{"digits after first rune", "let value2 = 1\n", "value2", "value2"},
		{"backtick escaped", "func `default`() {}\n", "`default`", "`default`"},
		{"unicode identifier", "let café = 1\n", "café", "café"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := common.NewSource(t.TempDir(), "n.swift", []byte(tc.source))
			at := strings.Index(tc.source, tc.anchor)
			if !assert.NotEqual(t, -1, at, "anchor should exist in fixture") {
				return
			}

			got := languages.Swift.NormalizeDefinitionRange(src, pointAt(src, at))

			if !assert.NotNil(t, got) {
				return
			}
			assert.Equal(t, tc.want, string(got.GetBytes()))
			assert.Equal(t, at+len(tc.want), got.End.BytePosition)
		})
	}
}

// Anything that is not an identifier is left alone; widening it would invent a
// span the server never claimed.
func TestSwiftNormalizeDefinitionRangeLeavesNonIdentifiersAlone(t *testing.T) {
	src := common.NewSource(t.TempDir(), "n.swift", []byte("let a = 1\n"))

	for _, at := range []int{5, len(src.Buffer)} {
		got := languages.Swift.NormalizeDefinitionRange(src, pointAt(src, at))

		if !assert.NotNil(t, got) {
			return
		}
		assert.Equal(t, got.Start.BytePosition, got.End.BytePosition,
			"a point that is not on an identifier stays a point")
	}
}

// A server that already sends the name's span must see it survive untouched
func TestNormalizeDefinitionRangeKeepsExistingSpans(t *testing.T) {
	for _, lang := range []struct {
		name string
		ls   languages.LanguageSupport
		file string
		body string
	}{
		{"swift", languages.Swift, "n.swift", "func widen() {}\n"},
		{"golang", languages.Golang, "n.go", "package main\n"},
		{"python", languages.Python, "n.py", "wide = 1\n"},
	} {
		t.Run(lang.name, func(t *testing.T) {
			src := common.NewSource(t.TempDir(), lang.file, []byte(lang.body))
			span, err := common.NewFileRangeAutoBytePosition(src, 0, 0, 0, 4)
			if !assert.NoError(t, err) {
				return
			}

			assert.Same(t, span, lang.ls.NormalizeDefinitionRange(src, span))
		})
	}
}

// A zero-width range must pass through rather than be widened by the wrong language
func TestNonSwiftNormalizeDefinitionRangeIsIdentity(t *testing.T) {
	src := common.NewSource(t.TempDir(), "n.go", []byte("package main\n"))
	point := pointAt(src, 8)

	assert.Same(t, point, languages.Golang.NormalizeDefinitionRange(src, point))
	assert.Same(t, point, languages.Python.NormalizeDefinitionRange(src, point))
}
