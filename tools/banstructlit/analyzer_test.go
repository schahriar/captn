package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), Analyzer, "github.com/schahriar/captn/pkg/sample")
}

func TestIgnoreDirectiveAbove(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want bool
	}{
		{
			name: "line comment",
			src: `package p
type T struct{}
func f() {
	//banstructlit:ignore
	_ = T{}
}`,
			want: true,
		},
		{
			name: "spaced line comment",
			src: `package p
type T struct{}
func f() {
	// banstructlit:ignore
	_ = T{}
}`,
			want: true,
		},
		{
			name: "block comment",
			src: `package p
type T struct{}
func f() {
	/* banstructlit:ignore */
	_ = T{}
}`,
			want: true,
		},
		{
			name: "blank line between comment and code",
			src: `package p
type T struct{}
func f() {
	// banstructlit:ignore

	_ = T{}
}`,
			want: false,
		},
		{
			name: "same line comment",
			src: `package p
type T struct{}
func f() {
	_ = T{} // banstructlit:ignore
}`,
			want: false,
		},
		{
			name: "different comment",
			src: `package p
type T struct{}
func f() {
	// ignore
	_ = T{}
}`,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "p.go", tt.src, parser.ParseComments)
			if err != nil {
				t.Fatal(err)
			}
			got := ignoreDirectiveAbove(file, fset, firstCompositeLit(t, file).Pos())
			if got != tt.want {
				t.Fatalf("ignoreDirectiveAbove() = %v, want %v", got, tt.want)
			}
		})
	}
}

func firstCompositeLit(t *testing.T, file *ast.File) *ast.CompositeLit {
	t.Helper()
	var lit *ast.CompositeLit
	ast.Inspect(file, func(n ast.Node) bool {
		if lit != nil {
			return false
		}
		if n, ok := n.(*ast.CompositeLit); ok {
			lit = n
			return false
		}
		return true
	})
	if lit == nil {
		t.Fatal("missing composite literal")
	}
	return lit
}
