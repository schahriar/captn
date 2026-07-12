package mcp

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/schahriar/captn/pkg/common"
)

func StartServer(ctx context.Context, addr string) error {
	server := mcp.NewServer(&mcp.Implementation{Name: "captn", Version: common.Version}, nil)

	defse, toolse := ToMCPTool(searchAndQueryTool)
	mcp.AddTool(server, defse, toolse)

	handler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{
		SessionTimeout: 30 * time.Minute,
	})

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
