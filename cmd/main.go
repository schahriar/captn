package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/alecthomas/kong"
	"github.com/schahriar/captn/pkg/initialize"
	"github.com/schahriar/captn/pkg/mcp"
	"github.com/schahriar/captn/pkg/providers"
	"github.com/schahriar/captn/pkg/server"
	"github.com/spf13/pflag"
)

var CLI struct {
	UseAPIKey bool `name:"use-api-key" help:"Pass ANTHROPIC_API_KEY through to claude instead of stripping it"`

	Search struct {
		Main    string `arg:"" name:"path" help:"Path to main file" type:"path"`
		Snippet string `arg:"snippet" name:"snippet" help:"Snippet to focus on in the main file" type:"string"`
		QueryID string `arg:"" optional:"" name:"queryId" help:"ID of the supported query to run against the matched snippets" type:"string"`
	} `cmd:"search" help:"Evaluate from main file"`

	Claude struct{} `cmd:"" help:"Run Claude Claude using Captn"`
}

func splitPassthroughArgs(args []string) ([]string, []string) {
	for i, arg := range args {
		if arg == "--" {
			// argv is already tokenized by the shell, so the tail passes through
			// untouched; re-joining and re-splitting would break args containing spaces
			return args[:i], args[i+1:]
		}
	}

	return args, nil
}

func main() {
	captnArgs, claudeArgs := splitPassthroughArgs(os.Args[1:])
	parser := kong.Must(&CLI)
	cli, err := parser.Parse(captnArgs)
	parser.FatalIfErrorf(err)

	providers.UseAPIKey = CLI.UseAPIKey

	switch cli.Command() {
	case "claude":
		flags := pflag.NewFlagSet("claude", pflag.ContinueOnError)
		flags.ParseErrorsAllowlist.UnknownFlags = true
		flags.SetOutput(io.Discard)

		print := flags.BoolP("print", "p", false, "")
		outputFormat := flags.String("output-format", "", "")

		if err := flags.Parse(claudeArgs); err != nil {
			log.Fatalf("failed to parse claude args: %v", err)
		}

		// Dedicated --print mode, no TUI just answer
		if *print || len(*outputFormat) > 0 {
			if err := initialize.Init(claudeArgs); err != nil {
				log.Fatalf("failed to initialize: %v", err)
			}
		} else {
			// TUI
			if err := initialize.InitWithTUI(context.Background(), claudeArgs); err != nil {
				log.Fatalf("failed to initialize with TUI: %v", err)
			}
		}
	case "search <path> <snippet>", "search <path> <snippet> <queryId>":
		ctx := context.Background()

		// banstructlit:ignore
		o, err := server.Search(ctx, mcp.SearchAndQueryInput{
			Include: cli.Args[1],
			Snippet: cli.Args[2],
			QueryID: CLI.Search.QueryID,
		})

		if err != nil {
			log.Fatalf("failed to run query: %v", err)
		}

		b, err := json.Marshal(o.Explanations)
		if err != nil {
			log.Fatalf("failed to marshal explanations: %v", err)
		}

		fmt.Println(string(b))
	default:
		panic(cli.Command())
	}
}
