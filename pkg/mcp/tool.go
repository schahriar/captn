package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Tool[In any, Out any] interface {
	Name() string
	Description() string
	Call(ctx context.Context, req *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error)
}

func ToMCPTool[In any, Out any](t Tool[In, Out]) (*mcp.Tool, mcp.ToolHandlerFor[In, Out]) {
	return &mcp.Tool{
		Name:        t.Name(),
		Description: t.Description(),
	}, t.Call
}
