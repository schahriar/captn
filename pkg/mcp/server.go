package mcp

import (
	"context"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/schahriar/captn/pkg/common"
)

func StartServer(ctx context.Context, addr string) error {
	server := mcp.NewServer(&mcp.Implementation{Name: "captn", Version: common.Version}, nil)

	def, tool := ToMCPTool(explainTool)
	mcp.AddTool(server, def, tool)

	def2, tool2 := ToMCPTool(searchAndExplainTool)
	mcp.AddTool(server, def2, tool2)

	handler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		return server
	}, nil)

	go func() {
		if err := http.ListenAndServe(addr, handler); err != nil {
			panic(fmt.Errorf("failed to start mcp server at (%v): %v", addr, err))
		}
	}()

	return nil
}

type ClaudeMCPServer struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

func NewClaudeMCPServer(serverType, url string) ClaudeMCPServer {
	return ClaudeMCPServer{
		Type: serverType,
		URL:  url,
	}
}

type ClaudeMCPConfig struct {
	MCPServers map[string]ClaudeMCPServer `json:"mcpServers"`
}

func NewClaudeMCPConfig(servers map[string]ClaudeMCPServer) ClaudeMCPConfig {
	return ClaudeMCPConfig{MCPServers: servers}
}
