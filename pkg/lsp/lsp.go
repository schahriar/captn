package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
)

type SpawnFunc func(ctx context.Context) (*ServerProcess, error)

type ServerProcess struct {
	Reader io.Reader
	Writer io.WriteCloser
	Wait   func() error
	Kill   func() error
}

type StartOptions struct {
	WorkspaceRoot         string
	ClientName            string
	ClientVersion         string
	Capabilities          map[string]any
	InitializationOptions any
	Spawn                 SpawnFunc
}

type Client struct {
	reader *bufio.Reader
	writer io.Writer

	writeMu sync.Mutex

	pendingMu sync.Mutex
	nextID    int64
	pending   map[int64]chan response

	closed chan struct{}
	proc   *ServerProcess
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

type LocationLink struct {
	OriginSelectionRange *Range `json:"originSelectionRange,omitempty"`
	TargetURI            string `json:"targetUri"`
	TargetRange          Range  `json:"targetRange"`
	TargetSelectionRange Range  `json:"targetSelectionRange"`
}

type SourcePoint struct {
	Row    int
	Column int
}

type SourceRange struct {
	Start SourcePoint
	End   SourcePoint
}

type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type ReferenceRequest struct {
	TextDocument       TextDocumentItem
	Range              SourceRange
	IncludeDeclaration bool
}

type DefinitionRequest struct {
	TextDocument TextDocumentItem
	Range        SourceRange
}

func Start(ctx context.Context, opts StartOptions) (*Client, error) {
	if opts.Spawn == nil {
		return nil, fmt.Errorf("missing LSP spawn function")
	}

	root := opts.WorkspaceRoot
	if root == "" {
		root = "."
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	proc, err := opts.Spawn(ctx)
	if err != nil {
		return nil, err
	}

	if proc.Reader == nil {
		return nil, fmt.Errorf("spawned LSP process has nil reader")
	}

	if proc.Writer == nil {
		return nil, fmt.Errorf("spawned LSP process has nil writer")
	}

	c := &Client{
		reader:  bufio.NewReader(proc.Reader),
		writer:  proc.Writer,
		pending: make(map[int64]chan response),
		closed:  make(chan struct{}),
		proc:    proc,
	}

	go c.readLoop()

	opts.WorkspaceRoot = absRoot

	if err := c.initialize(ctx, opts); err != nil {
		_ = c.Close(ctx)
		return nil, err
	}

	if err := c.Notify("initialized", map[string]any{}); err != nil {
		_ = c.Close(ctx)
		return nil, err
	}

	return c, nil
}

func (c *Client) References(ctx context.Context, req ReferenceRequest) ([]Location, error) {
	if req.TextDocument.URI == "" {
		return nil, fmt.Errorf("missing text document URI")
	}

	if req.TextDocument.LanguageID == "" {
		return nil, fmt.Errorf("missing text document language ID")
	}

	if req.TextDocument.Text == "" {
		return nil, fmt.Errorf("missing text document text")
	}

	position, err := PositionInsideRange(req.TextDocument.Text, req.Range)
	if err != nil {
		return nil, err
	}

	if err := c.OpenDocument(req.TextDocument); err != nil {
		return nil, err
	}

	var refs []Location

	err = c.Request(ctx, "textDocument/references", map[string]any{
		"textDocument": TextDocumentIdentifier{
			URI: req.TextDocument.URI,
		},
		"position": position,
		"context": map[string]any{
			"includeDeclaration": req.IncludeDeclaration,
		},
	}, &refs)
	if err != nil {
		return nil, err
	}

	return refs, nil
}

func (c *Client) Definition(ctx context.Context, req DefinitionRequest) ([]Location, error) {
	if req.TextDocument.URI == "" {
		return nil, fmt.Errorf("missing text document URI")
	}

	if req.TextDocument.LanguageID == "" {
		return nil, fmt.Errorf("missing text document language ID")
	}

	if req.TextDocument.Text == "" {
		return nil, fmt.Errorf("missing text document text")
	}

	position, err := PositionInsideRange(req.TextDocument.Text, req.Range)
	if err != nil {
		return nil, err
	}

	if err := c.OpenDocument(req.TextDocument); err != nil {
		return nil, err
	}

	var raw json.RawMessage

	err = c.Request(ctx, "textDocument/definition", map[string]any{
		"textDocument": TextDocumentIdentifier{
			URI: req.TextDocument.URI,
		},
		"position": position,
	}, &raw)
	if err != nil {
		return nil, err
	}

	return decodeDefinitionResult(raw)
}

func PositionInsideRange(text string, r SourceRange) (Position, error) {
	if r.Start.Row < 0 || r.Start.Column < 0 || r.End.Row < 0 || r.End.Column < 0 {
		return Position{}, fmt.Errorf("range cannot contain negative row or column")
	}

	if r.End.Row < r.Start.Row || r.End.Row == r.Start.Row && r.End.Column < r.Start.Column {
		return Position{}, fmt.Errorf("range end is before range start")
	}

	lines := strings.Split(text, "\n")

	if r.Start.Row >= len(lines) {
		return Position{}, fmt.Errorf("range start row out of bounds")
	}

	if r.End.Row >= len(lines) {
		return Position{}, fmt.Errorf("range end row out of bounds")
	}

	for row := r.Start.Row; row <= r.End.Row; row++ {
		line := strings.TrimSuffix(lines[row], "\r")

		startColumn := 0
		if row == r.Start.Row {
			startColumn = r.Start.Column
		}

		endColumn := utf16Len(line)
		if row == r.End.Row {
			endColumn = r.End.Column
		}

		if startColumn > endColumn {
			return Position{}, fmt.Errorf("range start column out of bounds")
		}

		if endColumn > utf16Len(line) {
			return Position{}, fmt.Errorf("range end column out of bounds")
		}

		startByte, err := byteOffsetForUTF16Column(line, startColumn)
		if err != nil {
			return Position{}, err
		}

		endByte, err := byteOffsetForUTF16Column(line, endColumn)
		if err != nil {
			return Position{}, err
		}

		for byteIndex, ch := range line[startByte:endByte] {
			if isIgnoredSelectionRune(ch) {
				continue
			}

			column, err := utf16ColumnAtByteOffset(line, startByte+byteIndex)
			if err != nil {
				return Position{}, err
			}

			return Position{
				Line:      row,
				Character: column,
			}, nil
		}
	}

	return Position{
		Line:      r.Start.Row,
		Character: r.Start.Column,
	}, nil
}

func (c *Client) OpenDocument(doc TextDocumentItem) error {
	if doc.URI == "" {
		return fmt.Errorf("missing text document URI")
	}

	if doc.LanguageID == "" {
		return fmt.Errorf("missing text document language ID")
	}

	return c.Notify("textDocument/didOpen", map[string]any{
		"textDocument": doc,
	})
}

func (c *Client) Request(ctx context.Context, method string, params any, result any) error {
	return c.request(ctx, method, params, result)
}

func (c *Client) Notify(method string, params any) error {
	return c.write(map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
}

func (c *Client) Close(ctx context.Context) error {
	_ = c.request(ctx, "shutdown", nil, nil)
	_ = c.Notify("exit", nil)

	if c.proc != nil && c.proc.Writer != nil {
		_ = c.proc.Writer.Close()
	}

	if c.proc != nil && c.proc.Wait != nil {
		return c.proc.Wait()
	}

	return nil
}

func (c *Client) initialize(ctx context.Context, opts StartOptions) error {
	clientName := opts.ClientName
	if clientName == "" {
		clientName = "lsp-client"
	}

	capabilities := opts.Capabilities
	if capabilities == nil {
		capabilities = DefaultCapabilities()
	}

	root := opts.WorkspaceRoot

	params := map[string]any{
		"processId": os.Getpid(),
		"rootUri":   FileURI(root),
		"workspaceFolders": []map[string]any{
			{
				"uri":  FileURI(root),
				"name": filepath.Base(root),
			},
		},
		"capabilities": capabilities,
		"clientInfo": map[string]any{
			"name":    clientName,
			"version": opts.ClientVersion,
		},
	}

	if opts.InitializationOptions != nil {
		params["initializationOptions"] = opts.InitializationOptions
	}

	var out any
	return c.request(ctx, "initialize", params, &out)
}

func (c *Client) request(ctx context.Context, method string, params any, result any) error {
	id := atomic.AddInt64(&c.nextID, 1)
	ch := make(chan response, 1)

	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()

	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}

	if err := c.write(msg); err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return err
	}

	select {
	case res := <-ch:
		if res.err != nil {
			return fmt.Errorf("lsp %s failed: %d %s", method, res.err.Code, res.err.Message)
		}

		if result == nil || len(res.result) == 0 || string(res.result) == "null" {
			return nil
		}

		return json.Unmarshal(res.result, result)

	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return ctx.Err()

	case <-c.closed:
		return io.ErrClosedPipe
	}
}

func (c *Client) write(v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}

	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if _, err := fmt.Fprintf(c.writer, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}

	_, err = c.writer.Write(body)
	return err
}

func (c *Client) readLoop() {
	defer close(c.closed)

	for {
		body, err := readFramedMessage(c.reader)
		if err != nil {
			c.failAll(err)
			return
		}

		msg := incomingMessage{}

		if err := json.Unmarshal(body, &msg); err != nil {
			continue
		}

		if msg.ID != nil && msg.Method != "" {
			c.respondToServerRequest(msg.ID, msg.Method, msg.Params)
			continue
		}

		if msg.ID == nil {
			continue
		}

		numericID := func(v any) (int64, bool) {
			switch x := v.(type) {
			case float64:
				return int64(x), true
			case int64:
				return x, true
			case int:
				return int64(x), true
			default:
				return 0, false
			}
		}

		id, ok := numericID(msg.ID)
		if !ok {
			continue
		}

		c.pendingMu.Lock()
		ch := c.pending[id]
		delete(c.pending, id)
		c.pendingMu.Unlock()

		if ch != nil {
			ch <- response{
				result: msg.Result,
				err:    msg.Error,
			}
		}
	}
}

func (c *Client) respondToServerRequest(id any, method string, params json.RawMessage) {
	var result any

	switch method {
	case "workspace/configuration":
		var req struct {
			Items []any `json:"items"`
		}

		_ = json.Unmarshal(params, &req)

		items := make([]any, len(req.Items))
		for i := range items {
			items[i] = map[string]any{}
		}

		result = items

	case "window/workDoneProgress/create":
		result = nil

	case "client/registerCapability":
		result = nil

	case "client/unregisterCapability":
		result = nil

	default:
		result = nil
	}

	_ = c.write(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
}

func (c *Client) failAll(err error) {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()

	for id, ch := range c.pending {
		delete(c.pending, id)
		ch <- response{
			err: &RPCError{
				Code:    -1,
				Message: err.Error(),
			},
		}
	}
}

func DefaultCapabilities() map[string]any {
	return map[string]any{
		"textDocument": map[string]any{
			"definition": map[string]any{
				"dynamicRegistration": false,
				"linkSupport":         true,
			},
			"references": map[string]any{
				"dynamicRegistration": false,
			},
			"synchronization": map[string]any{
				"didOpen": true,
				"didSave": true,
			},
		},
		"workspace": map[string]any{
			"workspaceFolders": true,
			"configuration":    true,
		},
	}
}

func FileURI(path string) string {
	abs, _ := filepath.Abs(path)
	abs = filepath.Clean(abs)

	if runtime.GOOS == "windows" {
		abs = "/" + filepath.ToSlash(abs)
	} else {
		abs = filepath.ToSlash(abs)
	}

	u := url.URL{
		Scheme: "file",
		Path:   abs,
	}

	return u.String()
}

type DocumentLink struct {
	Range  Range  `json:"range"`
	Target string `json:"target,omitempty"`
}

type DocumentLinkRequest struct {
	TextDocument TextDocumentItem
}

func (c *Client) DocumentLinks(ctx context.Context, req DocumentLinkRequest) ([]DocumentLink, error) {
	if req.TextDocument.URI == "" {
		return nil, fmt.Errorf("missing text document URI")
	}

	if req.TextDocument.LanguageID == "" {
		return nil, fmt.Errorf("missing text document language ID")
	}

	if req.TextDocument.Text == "" {
		return nil, fmt.Errorf("missing text document text")
	}

	if err := c.OpenDocument(req.TextDocument); err != nil {
		return nil, err
	}

	var links []DocumentLink

	err := c.Request(ctx, "textDocument/documentLink", map[string]any{
		"textDocument": TextDocumentIdentifier{
			URI: req.TextDocument.URI,
		},
	}, &links)
	if err != nil {
		return nil, err
	}

	return links, nil
}

func (c *Client) ImportTarget(ctx context.Context, doc TextDocumentItem, r Range) (string, error) {
	links, err := c.DocumentLinks(ctx, DocumentLinkRequest{
		TextDocument: doc,
	})
	if err != nil {
		return "", err
	}

	for _, link := range links {
		if rangesOverlap(link.Range, r) {
			return link.Target, nil
		}
	}

	return "", nil
}

func (c *Client) ImportDefinition(ctx context.Context, doc TextDocumentItem, importRange Range) ([]Location, error) {
	if doc.URI == "" {
		return nil, fmt.Errorf("missing text document URI")
	}

	if doc.LanguageID == "" {
		return nil, fmt.Errorf("missing text document language ID")
	}

	if doc.Text == "" {
		return nil, fmt.Errorf("missing text document text")
	}

	if err := c.OpenDocument(doc); err != nil {
		return nil, err
	}

	position := Position{
		Line:      importRange.Start.Line,
		Character: importRange.Start.Character + 1,
	}

	var raw json.RawMessage

	err := c.Request(ctx, "textDocument/definition", map[string]any{
		"textDocument": TextDocumentIdentifier{
			URI: doc.URI,
		},
		"position": position,
	}, &raw)
	if err != nil {
		return nil, err
	}

	return decodeDefinitionResult(raw)
}
