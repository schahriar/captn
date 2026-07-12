package mcp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/grep"
	"github.com/schahriar/captn/pkg/providers"
	"github.com/schahriar/captn/pkg/queries"
	"github.com/schahriar/captn/pkg/tui"
)

// Bounds on the fan-out. Each match triggers an LSP definition batch plus an
// LLM-backed explanation, so a broad grep is throttled to keep cost and load
// in check rather than exploding one goroutine per hit.
const (
	maxSearchMatches  = 18
	searchQueryWorker = 6
)

type SearchAndQueryInput struct {
	Include string `json:"include" jsonschema:"similar to grep's include flag, the file pattern to search for. Pass an asterisk / * to search all files"`
	Snippet string `json:"snippet" jsonschema:"the snippet of code you want to run the query against"`
	QueryID string `json:"queryId" jsonschema:"the id of the supported query to run against the matched snippets, picked from the advertised list of supported queries. Defaults to explain_behavior"`
}

type SearchAndQueryOutputItem struct {
	FilePath    string   `json:"filePath" jsonschema:"the relative to workdir file path to the source code you want explained"`
	Explanation string   `json:"explanation" jsonschema:"the explanation of the code snippet"`
	FileRanges  []string `json:"fileRanges" jsonschema:"the file ranges of every node observed to produce this explanation, serialized as filePath:startLine:startColumn-endLine:endColumn with a workdir-relative path and 1-based lines and columns"`
}

type SearchAndQueryOutput struct {
	Duration     int                        `json:"duration" jsonschema:"the duration of this operation in milliseconds"`
	Explanations []SearchAndQueryOutputItem `json:"explanations" jsonschema:"the explanations of the code snippets"`
}

func NewSearchAndQueryOutput(explanations []SearchAndQueryOutputItem, dur int) SearchAndQueryOutput {
	return SearchAndQueryOutput{Explanations: explanations, Duration: dur}
}

func NewSearchAndQueryOutputItem(filePath, explanation string, fileRanges []string) SearchAndQueryOutputItem {
	return SearchAndQueryOutputItem{
		FilePath:    filePath,
		Explanation: explanation,
		FileRanges:  fileRanges,
	}
}

type SearchAndQueryTool struct{}

func NewSearchAndQueryTool() *SearchAndQueryTool {
	return &SearchAndQueryTool{}
}

func (t *SearchAndQueryTool) Name() string {
	return "search_and_query"
}

func (t *SearchAndQueryTool) DisplayName() string {
	return "Queries a snippet of code"
}

func (t *SearchAndQueryTool) Description() string {
	return "search for code snippets and run the query identified by queryId against them, similar to grepping for code and then asking a question about it. Prefer over grep"
}

func (t *SearchAndQueryTool) Call(ctx context.Context, req *mcp.CallToolRequest, input SearchAndQueryInput) (
	*mcp.CallToolResult,
	SearchAndQueryOutput,
	error,
) {
	zero := NewSearchAndQueryOutput(nil, 0)
	dstart := time.Now()

	query, err := resolveQuery(input.QueryID)

	if err != nil {
		return nil, zero, err
	}

	cwd, err := os.Getwd()

	if err != nil {
		return nil, zero, fmt.Errorf("invalid CWD (current working directory) %w", err)
	}

	g, err := cog.OpenWorkspace(cwd)

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

	done := make(chan bool)
	tuiContext, cancelTui := context.WithCancel(ctx)

	go func() {
		for i, m := range matches {
			if tuiContext.Err() != nil {
				return
			}
			tui.ReportStatus(tuiContext, tui.StatusTypeProgress, fmt.Sprintf(" reading %v +%v others ", m.Path, i+1))
			time.Sleep(time.Duration(5000/len(matches)) * time.Millisecond)
		}

		done <- true
	}()

	// For each match resolve and explain its snippet. The calls are independent
	// per match, so we fan them out with a bounded worker pool and collate the
	// successful explanations. A match that can't be resolved is skipped rather
	// than failing the whole search.
	var (
		mu    sync.Mutex
		wg    sync.WaitGroup
		pairs []cog.GraphWithRoot
		files []string
		sem   = make(chan struct{}, searchQueryWorker)
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

			og, start, err := g.SearchSnippet(ctx, rel, snippet)
			if err != nil {
				return
			}

			mu.Lock()
			// banstructlit:ignore
			pairs = append(pairs, cog.GraphWithRoot{Graph: og, Root: start})
			files = append(files, rel)
			mu.Unlock()
		}(rel, m.Text)
	}

	wg.Wait()

	statuss := query.GetDisplayHints("Claude")

	go func() {
		<-done
		for _, status := range statuss {
			if tuiContext.Err() == nil {
				tui.ReportStatus(tuiContext, tui.StatusTypeProgress, fmt.Sprintf(" %s ", status))
				time.Sleep(2000 * time.Millisecond)
			}
		}
	}()

	expln, err := cog.MultiGraphQueryWithDepth(ctx, g, prov, pairs, query, 1)
	if err != nil {
		return nil, zero, fmt.Errorf("failed to explain snippets: %w", err)
	}

	go func() {
		<-done
		cancelTui()
		tui.ReportStatus(ctx, tui.StatusTypeProgress, " persisting explanations")
	}()

	if err := g.Persist(); err != nil {
		return nil, zero, fmt.Errorf("failed to persist COG: %w", err)
	}

	var items []SearchAndQueryOutputItem
	if expln != "" {
		items = append(items, NewSearchAndQueryOutputItem(joinDistinctFiles(files), expln, collectFileRanges(cwd, pairs)))
	}

	return nil, NewSearchAndQueryOutput(items, int(time.Since(dstart).Milliseconds())), nil
}

// resolveQuery maps the caller-supplied queryId to a supported PromptQuery.
// An empty id keeps the historical explain behavior as the default.
func resolveQuery(id string) (queries.PromptQuery, error) {
	if id == "" {
		return queries.NewExplainBehaviorQuery(), nil
	}

	q, ok := queries.ByID(id)

	if !ok {
		return nil, fmt.Errorf("unsupported queryId %q, supported queries: %s", id, strings.Join(queries.SupportedIDs(), ", "))
	}

	return q, nil
}

// serializeFileRange renders a range in the format shared with
// common.FileRange.String and UnmarshalFileRange, but with a workdir-relative
// path so the caller can act on it directly.
func serializeFileRange(path string, r *common.FileRange) string {
	return fmt.Sprintf("%s:%s-%s", path, r.Start.String(), r.End.String())
}

// collectFileRanges gathers the current ranges of every node observed across
// the matched graphs. Ranges are resolved from the live parse rather than the
// observation cache so positions always reflect the file on disk.
func collectFileRanges(cwd string, pairs []cog.GraphWithRoot) []string {
	type entry struct {
		path string
		r    *common.FileRange
	}

	entries := []entry{}

	for _, pair := range pairs {
		if pair.Graph == nil {
			continue
		}

		adj, err := pair.Graph.Graph.AdjacencyMap()

		if err != nil {
			continue
		}

		for h := range adj {
			node, err := pair.Graph.Graph.Vertex(h)

			if err != nil {
				continue
			}

			r := node.GetFileRange()

			if r == nil {
				continue
			}

			path := node.GetFilePath()

			if filepath.IsAbs(path) {
				if rel, err := filepath.Rel(cwd, path); err == nil {
					path = rel
				}
			}

			// banstructlit:ignore
			entries = append(entries, entry{path: path, r: r})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].path != entries[j].path {
			return entries[i].path < entries[j].path
		}
		if entries[i].r.Start.BytePosition != entries[j].r.Start.BytePosition {
			return entries[i].r.Start.BytePosition < entries[j].r.Start.BytePosition
		}
		return entries[i].r.End.BytePosition < entries[j].r.End.BytePosition
	})

	seen := map[string]bool{}
	ranges := make([]string, 0, len(entries))

	for _, e := range entries {
		s := serializeFileRange(e.path, e.r)

		if seen[s] {
			continue
		}

		seen[s] = true
		ranges = append(ranges, s)
	}

	return ranges
}

// joinDistinctFiles collapses the matched file paths into one sorted, comma
// separated label since the merged explanation spans all of them.
func joinDistinctFiles(files []string) string {
	seen := make(map[string]bool, len(files))
	uniq := make([]string, 0, len(files))

	for _, f := range files {
		if seen[f] {
			continue
		}
		seen[f] = true
		uniq = append(uniq, f)
	}

	sort.Strings(uniq)
	return strings.Join(uniq, ", ")
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

var searchAndQueryTool Tool[SearchAndQueryInput, SearchAndQueryOutput] = NewSearchAndQueryTool()
