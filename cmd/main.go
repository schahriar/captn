package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime/trace"

	"github.com/alecthomas/kong"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/providers"
)

var CLI struct {
	Run struct {
		Main    string `arg:"" name:"path" help:"Path to main file" type:"path"`
		Snippet string `arg:"snippet" name:"snippet" help:"Snippet to focus on in the main file" type:"string"`
	} `cmd:"" help:"Evaluate from main file"`
}

func main() {
	cli := kong.Parse(&CLI)
	switch cli.Command() {
	case "run <path> <snippet>":
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

		ctx, task := trace.NewTask(ctx, "loadCOG")
		cwd, err := os.Getwd()

		if err != nil {
			panic(fmt.Errorf("invalid CWD (current working directory) %w", err))
		}

		g, err := cog.OpenCOG(cwd)

		if err != nil {
			panic(err)
		}

		task.End()

		prov := &providers.ClaudeCodeProvider{}

		og, start, err := g.QuerySnippet(ctx, cli.Args[1], cli.Args[2])
		if err != nil {
			panic(err)
		}

		expln, err := og.ExplainWithDepth(ctx, g, prov, start, 1)

		if err != nil {
			panic(err)
		}

		fmt.Println(expln)

		if err := g.Persist(); err != nil {
			panic(err)
		}
	default:
		panic(cli.Command())
	}
}
