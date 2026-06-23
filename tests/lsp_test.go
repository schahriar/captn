package tests_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/lsp"
	"github.com/stretchr/testify/assert"
)

func TestLSPDefinitionsReturnsLocationsByFileRange(t *testing.T) {
	ctx := context.Background()
	text := "package main\n\nfunc main() {\n\tfmt.Println(value)\n}\n"
	src := common.NewSource(".", "main.go", []byte(text))

	first, err := common.NewFileRangeAutoBytePosition(src, 2, 5, 2, 9)
	assert.NoError(t, err)

	second, err := common.NewFileRangeAutoBytePosition(src, 3, 13, 3, 18)
	assert.NoError(t, err)

	serverReader, clientWriter := io.Pipe()
	clientReader, serverWriter := io.Pipe()

	go serveDefinitionRequests(t, serverReader, serverWriter)

	client, err := lsp.Start(ctx, lsp.NewStartOptions(".", "test-client", "0.0.0", func(ctx context.Context) (*lsp.ServerProcess, error) {
		return lsp.NewServerProcess(clientReader, clientWriter, nil, nil), nil
	}))
	assert.NoError(t, err)

	got, err := client.Definitions(ctx, lsp.NewDefinitionBatchRequest(
		lsp.NewTextDocumentItem(lsp.FileURI("/workspace/main.go"), "go", 1, text),
		[]*common.FileRange{
			first,
			second,
		},
	))
	assert.NoError(t, err)

	assert.Len(t, got, 2)
	assert.Equal(t, []lsp.Location{
		lsp.NewLocation("file:///workspace/defs.go", lsp.Range{
			Start: lsp.NewPosition(10, 1),
			End:   lsp.NewPosition(10, 4),
		}),
	}, got[first])
	assert.Equal(t, []lsp.Location{
		lsp.NewLocation("file:///workspace/defs.go", lsp.Range{
			Start: lsp.NewPosition(20, 2),
			End:   lsp.NewPosition(20, 6),
		}),
	}, got[second])

	assert.NoError(t, clientWriter.Close())
	assert.NoError(t, serverWriter.Close())
}

func serveDefinitionRequests(t *testing.T, r io.Reader, w io.Writer) {
	t.Helper()

	br := bufio.NewReader(r)
	requests := 0

	for requests < 2 {
		body, err := readTestFramedMessage(br)
		if !assert.NoError(t, err) {
			return
		}

		var msg struct {
			ID     int64           `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if !assert.NoError(t, json.Unmarshal(body, &msg)) {
			return
		}

		if msg.Method == "initialize" {
			writeTestResponse(t, w, msg.ID, map[string]any{
				"capabilities": map[string]any{},
			})
			continue
		}

		if msg.ID == 0 {
			assert.Contains(t, []string{"initialized", "textDocument/didOpen"}, msg.Method)
			continue
		}

		assert.Equal(t, "textDocument/definition", msg.Method)

		var params struct {
			Position lsp.Position `json:"position"`
		}
		if !assert.NoError(t, json.Unmarshal(msg.Params, &params)) {
			return
		}

		var loc lsp.Location
		switch requests {
		case 0:
			assert.Equal(t, lsp.NewPosition(2, 5), params.Position)
			loc = lsp.NewLocation("file:///workspace/defs.go", lsp.Range{
				Start: lsp.NewPosition(10, 1),
				End:   lsp.NewPosition(10, 4),
			})
		case 1:
			assert.Equal(t, lsp.NewPosition(3, 13), params.Position)
			loc = lsp.NewLocation("file:///workspace/defs.go", lsp.Range{
				Start: lsp.NewPosition(20, 2),
				End:   lsp.NewPosition(20, 6),
			})
		}

		writeTestResponse(t, w, msg.ID, loc)
		requests++
	}
}

func readTestFramedMessage(r *bufio.Reader) ([]byte, error) {
	contentLength := -1

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}

		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		if strings.EqualFold(name, "Content-Length") {
			_, err := fmt.Sscanf(strings.TrimSpace(value), "%d", &contentLength)
			if err != nil {
				return nil, err
			}
		}
	}

	if contentLength < 0 {
		return nil, fmt.Errorf("missing Content-Length")
	}

	body := make([]byte, contentLength)
	_, err := io.ReadFull(r, body)
	return body, err
}

func writeTestResponse(t *testing.T, w io.Writer, id int64, result any) {
	t.Helper()

	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
	assert.NoError(t, err)

	_, err = fmt.Fprintf(w, "Content-Length: %d\r\n\r\n%s", len(body), bytes.TrimSpace(body))
	assert.NoError(t, err)
}
