package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"
	"github.com/goccy/go-yaml"
	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/languages"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_golang "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

var CLI struct {
	Run struct {
		Main string `arg:"" name:"path" help:"Path to main file" type:"path"`
	} `cmd:"" help:"Evaluate from main file"`
}

func x(v string) int {
	return 0
}

func main() {
	ctx := kong.Parse(&CLI)
	switch ctx.Command() {
	case "run <path>":
		dir, err := os.Getwd()

		if err != nil {
			panic(fmt.Errorf("Invalid CWD (current working directory) %w", err))
		}

		path := filepath.Join(dir, ctx.Args[1])

		src, err := common.NewSourceFromFile(path)

		if err != nil {
			panic(fmt.Errorf("Failed to read main file", err))
		}

		tsp := tree_sitter.NewParser()
		defer tsp.Close()
		tsp.SetLanguage(tree_sitter.NewLanguage(tree_sitter_golang.Language()))

		tree := tsp.Parse(src.Buffer, nil)
		defer tree.Close()

		tree.RootNode().Range()

		root, err := languages.Golang.Parse(src, tree)

		if err != nil {
			panic(fmt.Errorf("Failed to parse Go code: %w", err))
		}

		dbg := root.Debug()
		ser, _ := yaml.Marshal(dbg)

		os.WriteFile("./out.yaml", ser, 0655)
	default:
		panic(ctx.Command())
	}
}
