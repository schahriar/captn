package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/schahriar/captn/pkg/mcp"
)

func Search(ctx context.Context, input mcp.SearchAndExplainInput) (mcp.SearchAndExplainOutput, error) {
	return call[mcp.SearchAndExplainInput, mcp.SearchAndExplainOutput](ctx, mcp.NewSearchAndExplainTool().Name(), input)
}

func call[In any, Out any](ctx context.Context, task string, input In) (Out, error) {
	var out Out

	body, err := json.Marshal(input)
	if err != nil {
		return out, err
	}

	url := fmt.Sprintf("http://127.0.0.1:%d%s/%s", Port, taskPath, task)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return out, fmt.Errorf("captn server is not reachable on port %d, run `captn claude` first: %w", Port, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return out, fmt.Errorf("captn server returned %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}

	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return out, fmt.Errorf("invalid response from captn server: %w", err)
	}

	return out, nil
}
