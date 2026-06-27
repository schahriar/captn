package mcp

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/providers"
)

type ExplainInput struct {
	FilePath string `json:"filePath" jsonschema:"the relative to workdir file path to the source code you want explained"`
	Snippet  string `json:"snippet" jsonschema:"the snippet of code you want explained"`
}

type ExplainOutput struct {
	Duration    int    `json:"duration" jsonschema:"the duration of this operation in milliseconds"`
	Explanation string `json:"explanation" jsonschema:"the explanation of the code snippet"`
}

func NewExplainOutput(explanation string, duration int) ExplainOutput {
	return ExplainOutput{Explanation: explanation, Duration: duration}
}

type ExplainTool struct{}

func NewExplainTool() *ExplainTool {
	return &ExplainTool{}
}

func (t *ExplainTool) Name() string {
	return "explain"
}

func (t *ExplainTool) Description() string {
	return "explain a snippet of code in a file"
}

func (t *ExplainTool) Call(ctx context.Context, req *mcp.CallToolRequest, input ExplainInput) (
	*mcp.CallToolResult,
	ExplainOutput,
	error,
) {
	zero := NewExplainOutput("", 0)

	dstart := time.Now()
	cwd, err := os.Getwd()

	if err != nil {
		return nil, zero, fmt.Errorf("invalid CWD (current working directory) %w", err)
	}

	g, err := cog.OpenCOG(cwd)

	if err != nil {
		return nil, zero, err
	}

	prov := providers.NewClaudeCodeProvider()

	og, start, err := g.QuerySnippet(ctx, input.FilePath, input.Snippet)
	if err != nil {
		return nil, zero, fmt.Errorf("failed to query snippet: %w", err)
	}

	expln, err := og.ExplainWithDepth(ctx, g, prov, start, 1)

	if err != nil {
		return nil, zero, fmt.Errorf("failed to explain snippet: %w", err)
	}

	go func() {
		if err := g.Persist(); err != nil {
			// TODO: Report this error to the user in a better way. For now, we just log it.
			// But the logs override the claude stdout
			fmt.Printf("failed to persist COG: %v\n", err)
		}
	}()

	dur := int(time.Since(dstart).Milliseconds())

	return nil, NewExplainOutput(expln, dur), nil
}

var explainTool Tool[ExplainInput, ExplainOutput] = NewExplainTool()
