package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/grep"
	"github.com/schahriar/captn/pkg/providers"
)

// Bounds on the fan-out. Each match triggers an LSP definition batch plus an
// LLM-backed explanation, so a broad grep is throttled to keep cost and load
// in check rather than exploding one goroutine per hit.
const (
	maxSearchMatches    = 32
	searchExplainWorker = 8
)

type SearchAndExplainInput struct {
	Include string `json:"include" jsonschema:"similar to grep's include flag, the file pattern to search for. Pass an asterisk / * to search all files"`
	Snippet string `json:"snippet" jsonschema:"the snippet of code you want explained"`
}

type SearchAndExplainOutputItem struct {
	FilePath    string `json:"filePath" jsonschema:"the relative to workdir file path to the source code you want explained"`
	Explanation string `json:"explanation" jsonschema:"the explanation of the code snippet"`
}

type SearchAndExplainOutput struct {
	Explanations []SearchAndExplainOutputItem `json:"explanations" jsonschema:"the explanations of the code snippets"`
}

func NewSearchAndExplainOutput(explanations []SearchAndExplainOutputItem) SearchAndExplainOutput {
	return SearchAndExplainOutput{Explanations: explanations}
}

func NewSearchAndExplainOutputItem(filePath, explanation string) SearchAndExplainOutputItem {
	return SearchAndExplainOutputItem{
		FilePath:    filePath,
		Explanation: explanation,
	}
}

type SearchAndExplainTool struct{}

func NewSearchAndExplainTool() *SearchAndExplainTool {
	return &SearchAndExplainTool{}
}

func (t *SearchAndExplainTool) Name() string {
	return "search_and_explain"
}

func (t *SearchAndExplainTool) Description() string {
	return "search for code snippets and explain them, similar to grepping for code and then asking for an explanation of the code snippet. Prefer over grep"
}

func (t *SearchAndExplainTool) Call(ctx context.Context, req *mcp.CallToolRequest, input SearchAndExplainInput) (
	*mcp.CallToolResult,
	SearchAndExplainOutput,
	error,
) {
	zero := NewSearchAndExplainOutput(nil)
	cwd, err := os.Getwd()

	if err != nil {
		return nil, zero, fmt.Errorf("invalid CWD (current working directory) %w", err)
	}

	g, err := cog.OpenCOG(cwd)

	if err != nil {
		return nil, zero, err
	}

	prov := providers.NewClaudeCodeProvider()

	// Find the file paths and snippets matching the input. An empty include
	// searches every file, otherwise we constrain by the grep include pattern.
	var matches []grep.Match
	if input.Include == "" {
		matches, err = grep.Search(ctx, cwd, input.Snippet)
	} else {
		matches, err = grep.SearchSource(ctx, cwd, input.Include, input.Snippet)
	}

	if err != nil {
		return nil, zero, fmt.Errorf("failed to search source: %w", err)
	}

	matches = dedupeMatches(matches)
	if len(matches) > maxSearchMatches {
		matches = matches[:maxSearchMatches]
	}

	// For each match resolve and explain its snippet. The calls are independent
	// per match, so we fan them out with a bounded worker pool and collate the
	// successful explanations. A match that can't be resolved is skipped rather
	// than failing the whole search.
	var (
		mu    sync.Mutex
		wg    sync.WaitGroup
		items []SearchAndExplainOutputItem
		sem   = make(chan struct{}, searchExplainWorker)
	)

	for _, m := range matches {
		rel, err := filepath.Rel(cwd, m.Path)
		if err != nil {
			rel = m.Path
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(rel, snippet string) {
			defer wg.Done()
			defer func() { <-sem }()

			og, start, err := g.QuerySnippet(ctx, rel, snippet)
			if err != nil {
				return
			}

			expln, err := og.ExplainWithDepth(ctx, g, prov, start, 1)
			if err != nil {
				return
			}

			mu.Lock()
			items = append(items, NewSearchAndExplainOutputItem(rel, expln))
			mu.Unlock()
		}(rel, m.Text)
	}

	wg.Wait()

	if err := g.Persist(); err != nil {
		return nil, zero, fmt.Errorf("failed to persist COG: %w", err)
	}

	return nil, NewSearchAndExplainOutput(items), nil
}

// dedupeMatches collapses matches that share a file path and matched text since
// they resolve to the same snippet and explaining them more than once is wasted
// LSP and LLM work.
func dedupeMatches(matches []grep.Match) []grep.Match {
	seen := make(map[string]bool, len(matches))
	deduped := make([]grep.Match, 0, len(matches))

	for _, m := range matches {
		key := fmt.Sprintf("%s\x00%s", m.Path, m.Text)
		if seen[key] {
			continue
		}
		seen[key] = true
		deduped = append(deduped, m)
	}

	return deduped
}

var searchAndExplainTool Tool[SearchAndExplainInput, SearchAndExplainOutput] = NewSearchAndExplainTool()
