package mcp

import (
	"context"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/providers"
)

type ExplainInput struct {
	FilePath string `json:"filePath" jsonschema:"the relative to workdir file path to the source code you want explained"`
	Snippet  string `json:"snippet" jsonschema:"the snippet of code you want explained"`
}

type ExplainOutput struct {
	Explanation string `json:"explanation" jsonschema:"the explanation of the code snippet"`
}

func NewExplainOutput(explanation string) ExplainOutput {
	return ExplainOutput{Explanation: explanation}
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
	zero := NewExplainOutput("")
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

	if err := g.Persist(); err != nil {
		return nil, zero, fmt.Errorf("failed to persist COG: %w", err)
	}

	return nil, NewExplainOutput(expln), nil
}

var explainTool Tool[ExplainInput, ExplainOutput] = NewExplainTool()
