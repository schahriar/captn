package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime/trace"

	"github.com/alecthomas/kong"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/mcp"
	"github.com/schahriar/captn/pkg/providers"
)

var CLI struct {
	Run struct {
		Main    string `arg:"" name:"path" help:"Path to main file" type:"path"`
		Snippet string `arg:"snippet" name:"snippet" help:"Snippet to focus on in the main file" type:"string"`
	} `cmd:"" help:"Evaluate from main file"`

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
		ctx := context.Background()
		addr := "localhost:10580"

		if err := mcp.StartServer(ctx, addr); err != nil {
			log.Fatalf("failed to start MCP server: %v", err)
		}

		conf, err := json.Marshal(mcp.NewClaudeMCPConfig(map[string]mcp.ClaudeMCPServer{
			"captn": mcp.NewClaudeMCPServer("http", fmt.Sprintf("http://%s", addr)),
		}))

		if err != nil {
			log.Fatalf("failed to marshal MCP config: %v", err)
		}

		args := []string{"--append-system-prompt", `
		"Instead of calling Read tool, use the MCP "explain" tool.
		Always use the MCP "explain" or "search_and_explain" tool instead of grep or grep-equivalent tools.
		YOU SHOULD NEVER USE grep directly anymore, just use the MCP "search_and_explain" tool instead.
		`, "--mcp-config", string(conf)}
		args = append(args, claudeArgs...)

		cmd := exec.Command("claude", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin

		if err := cmd.Run(); err != nil {
			log.Fatalf("failed to run claude: %v", err)
		}

		break
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

		prov := providers.NewClaudeCodeProvider()

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
