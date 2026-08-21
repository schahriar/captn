// treedump prints the tree-sitter grammar tree captn sees for a file, one
// node per line with kind, field name and range, and the source text on
// leaves. Use it to probe grammar shapes before mapping them in a
// transformer: go run ./tools/treedump <file>
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/schahriar/captn/pkg/languages"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: treedump <file>")
		os.Exit(1)
	}

	path := os.Args[1]

	lang, ok := languages.ForExtension(filepath.Ext(path))

	if !ok {
		fmt.Fprintf(os.Stderr, "treedump: unsupported extension %q\n", filepath.Ext(path))
		os.Exit(1)
	}

	buf, err := os.ReadFile(path)

	if err != nil {
		fmt.Fprintln(os.Stderr, "treedump:", err)
		os.Exit(1)
	}

	parser := tree_sitter.NewParser()
	defer parser.Close()
	parser.SetLanguage(lang.GetTreeSitterLanguage())

	tree := parser.Parse(buf, nil)
	defer tree.Close()

	dump(buf, tree.RootNode(), "", 0)
}

func dump(buf []byte, node *tree_sitter.Node, field string, depth int) {
	label := node.Kind()

	if !node.IsNamed() {
		label = fmt.Sprintf("%q", label)
	}

	if field != "" {
		label = field + ": " + label
	}

	r := node.Range()
	line := fmt.Sprintf("%s%s [%d:%d-%d:%d]", strings.Repeat("  ", depth), label,
		r.StartPoint.Row, r.StartPoint.Column, r.EndPoint.Row, r.EndPoint.Column)

	if node.ChildCount() == 0 && node.IsNamed() {
		line += fmt.Sprintf(" %q", node.Utf8Text(buf))
	}

	fmt.Println(line)

	cursor := node.Walk()
	defer cursor.Close()

	for ok := cursor.GotoFirstChild(); ok; ok = cursor.GotoNextSibling() {
		dump(buf, cursor.Node(), cursor.FieldName(), depth+1)
	}
}
