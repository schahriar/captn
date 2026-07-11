package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/alecthomas/kong"
	"github.com/schahriar/captn/pkg/mcp"
	"github.com/schahriar/captn/pkg/server"
	"github.com/schahriar/captn/pkg/tui"
)

var CLI struct {
	Search struct {
		Main    string `arg:"" name:"path" help:"Path to main file" type:"path"`
		Snippet string `arg:"snippet" name:"snippet" help:"Snippet to focus on in the main file" type:"string"`
	} `cmd:"search" help:"Evaluate from main file"`

	Claude struct{} `cmd:"" help:"Run Claude Claude using Captn"`
}

func splitPassthroughArgs(args []string) ([]string, []string) {
	for i, arg := range args {
		if arg == "--" {
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

	switch cli.Command() {
	case "claude":
		srv := server.NewServer()
		if err := srv.Listen(); err != nil {
			log.Fatalf("failed to start captn server: %v", err)
		}
		defer srv.Close()

		args := []string{"--append-system-prompt", `
Instead of calling Read tool, use the "captn" binary.
Replace all calls to grep and reads with "captn search <path> <snippet>"
Where path can be a glob pattern like *.go or a specific file path, and snippet is the code snippet you want explained.
YOU SHOULD NEVER USE grep directly anymore, just use captn
		`}
		args = append(args, claudeArgs...)

		cmd := exec.Command("claude", args...)

		overlay, err := tui.NewOverlay(cmd)
		if err != nil {
			panic(err)
		}

		ctx := tui.WithStatusProvider(context.Background(), overlay)
		srv.Serve(ctx)

		go func() {
			time.Sleep(1000 * time.Millisecond)
			loader := tui.NewLoader()
			overlay.SetStatus(
				tui.Decorate(
					tui.Group(
						tui.Text(" captn "),
						loader,
					),
					tui.ShimmerColor(tui.NewRGB(70, 130, 220), tui.NewRGB(180, 220, 255)),
				),
			)

			overlay.SetSubStatus(
				tui.Group(
					tui.Text(tui.Dim("Ask anything and claude will coordinate with captn")),
				),
			)

			time.Sleep(5 * time.Second)

			overlay.SetSubStatus(tui.Group()) // Reset substatus after 5 seconds
			overlay.Hide()
		}()

		if err := overlay.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				os.Exit(exitErr.ExitCode())
			}
			panic(err)
		}
	case "search <path> <snippet>":
		ctx := context.Background()

		// banstructlit:ignore
		o, err := server.Search(ctx, mcp.SearchAndExplainInput{
			Include: cli.Args[1],
			Snippet: cli.Args[2],
		})

		if err != nil {
			log.Fatalf("failed to explain: %v", err)
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
