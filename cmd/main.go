package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/trace"

	"github.com/alecthomas/kong"
	"github.com/goccy/go-yaml"
	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/languages"
	"github.com/schahriar/captn/pkg/lsp"
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
	cli := kong.Parse(&CLI)
	switch cli.Command() {
	case "run <path>":
		f, err := os.Create("trace.out")
		if err != nil {
			log.Fatalf("failed to create trace output file: %v", err)
		}
		defer func() {
			if err := f.Close(); err != nil {
				log.Fatalf("failed to close trace file: %v", err)
			}
		}()

		if err := trace.Start(f); err != nil {
			log.Fatalf("failed to start trace: %v", err)
		}

		defer trace.Stop()

		ctx := context.Background()

		ctx, task := trace.NewTask(ctx, "loadFile")
		dir, err := os.Getwd()

		if err != nil {
			panic(fmt.Errorf("Invalid CWD (current working directory) %w", err))
		}

		path := filepath.Join(dir, cli.Args[1])

		src, err := common.NewSourceFromFile(path)

		if err != nil {
			panic(fmt.Errorf("Failed to read main file", err))
		}

		task.End()

		ctx, task = trace.NewTask(ctx, "treeSitterParse")

		tsp := tree_sitter.NewParser()
		defer tsp.Close()
		tsp.SetLanguage(tree_sitter.NewLanguage(tree_sitter_golang.Language()))

		tree := tsp.Parse(src.Buffer, nil)
		defer tree.Close()

		task.End()

		ctx, task = trace.NewTask(ctx, "languageParse")

		root, err := languages.Golang.Parse(ctx, src, tree)

		if err != nil {
			panic(fmt.Errorf("Failed to parse Go code: %w", err))
		}

		task.End()

		ctx, task = trace.NewTask(ctx, "LSPServer")

		reg := trace.StartRegion(ctx, "LSPServerLoad")

		client, err := lsp.Start(ctx, lsp.StartOptions{
			WorkspaceRoot: dir,
			ClientName:    "captn-lsp-client",
			ClientVersion: "0.1.0",
			Spawn:         languages.Golang.NewLSPServer,
		})

		if err != nil {
			panic(err)
		}

		reg.End()

		defer client.Close(ctx)

		reg = trace.StartRegion(ctx, "LSPQuery")

		impVis := &ast.ImportVisitor{}

		root.Accept(impVis)

		impLoc := []lsp.Location{}

		for _, imp := range impVis.Imports {
			pos := imp.GetPosition()

			fmt.Println(imp.Node.GetPosition().String())

			fmt.Println(imp.Reference, imp.Namespace)

			refs, err := client.ImportDefinition(ctx, lsp.TextDocumentItem{
				URI:        lsp.FileURI(path),
				LanguageID: "go",
				Version:    1,
				Text:       string(src.Buffer),
			}, lsp.Range{
				Start: lsp.Position{Line: pos.Start.Line, Character: pos.Start.Column},
				End:   lsp.Position{Line: pos.End.Line, Character: pos.End.Column},
			})

			if err != nil {
				panic(err)
			}

			for _, ref := range refs {
				impLoc = append(impLoc, ref)
			}
		}

		fmt.Println(impLoc)

		reg.End()
		task.End()

		dbg := root.Debug()
		ser, _ := yaml.Marshal(dbg)

		os.WriteFile("./out.yaml", ser, 0655)
	default:
		panic(cli.Command())
	}
}
