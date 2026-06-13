package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"

	"github.com/dominikbraun/graph"
	"github.com/dominikbraun/graph/draw"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/common"
)

type ClaudeCodeProvider struct{}

type ClaudeResult struct {
	Type           string `json:"type"`
	Subtype        string `json:"subtype"`
	IsError        bool   `json:"is_error"`
	APIErrorStatus *int   `json:"api_error_status"`

	DurationMS    int `json:"duration_ms"`
	DurationAPIMS int `json:"duration_api_ms"`
	TTFTMS        int `json:"ttft_ms"`
	NumTurns      int `json:"num_turns"`

	Result string `json:"result"`

	StopReason string `json:"stop_reason"`
	SessionID  string `json:"session_id"`
	UUID       string `json:"uuid"`

	TotalCostUSD float64 `json:"total_cost_usd"`

	Usage      ClaudeUsage                 `json:"usage"`
	ModelUsage map[string]ClaudeModelUsage `json:"modelUsage"`

	PermissionDenials []json.RawMessage `json:"permission_denials"`

	TerminalReason string `json:"terminal_reason"`
	FastModeState  string `json:"fast_mode_state"`
}

type ClaudeUsage struct {
	InputTokens              int `json:"input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	OutputTokens             int `json:"output_tokens"`

	ServerToolUse ClaudeServerToolUse `json:"server_tool_use"`

	ServiceTier  string `json:"service_tier"`
	InferenceGeo string `json:"inference_geo"`

	CacheCreation ClaudeCacheCreation `json:"cache_creation"`

	Iterations []ClaudeIterationUsage `json:"iterations"`
}

type ClaudeServerToolUse struct {
	WebSearchRequests int `json:"web_search_requests"`
	WebFetchRequests  int `json:"web_fetch_requests"`
}

type ClaudeCacheCreation struct {
	Ephemeral1HInputTokens int `json:"ephemeral_1h_input_tokens"`
	Ephemeral5MInputTokens int `json:"ephemeral_5m_input_tokens"`
}

type ClaudeIterationUsage struct {
	Type string `json:"type"`

	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`

	CacheCreation ClaudeCacheCreation `json:"cache_creation"`
}

type ClaudeModelUsage struct {
	InputTokens              int `json:"inputTokens"`
	OutputTokens             int `json:"outputTokens"`
	CacheReadInputTokens     int `json:"cacheReadInputTokens"`
	CacheCreationInputTokens int `json:"cacheCreationInputTokens"`

	WebSearchRequests int `json:"webSearchRequests"`

	CostUSD         float64 `json:"costUSD"`
	ContextWindow   int     `json:"contextWindow"`
	MaxOutputTokens int     `json:"maxOutputTokens"`
}

func QueryClaudeCode[T common.ObservationSchemaType](ctx context.Context, systemPrompt string, prompt string, effort string) (scma T, err error) {
	sscma, err := scma.Serialize()

	if err != nil {
		return scma, err
	}

	// TODO: Check for claude binary
	cmd := exec.CommandContext(ctx,
		"claude",
		"--no-session-persistence",
		"--print",
		"--effort", effort,
		"-p", prompt,
		"--output-format", "json",
		"--tools", "Read", // Only allow file reads
		"--system-prompt", systemPrompt,
		"--json-schema", sscma,
	)

	out, err := cmd.CombinedOutput()

	if err != nil {
		return scma, fmt.Errorf("failed to run claude code with error %w", err)
	}

	res := &ClaudeResult{}

	if err := json.Unmarshal(out, &res); err != nil {
		return scma, fmt.Errorf("unexpected response from claude with error %w \n Received: %s", err, out)
	}

	if err := json.Unmarshal([]byte(res.Result), &scma); err != nil {
		return scma, fmt.Errorf("structured output did not match expected schema with error %w \n Received: %s", err, res.Result)
	}

	return scma, nil
}

func (p *ClaudeCodeProvider) GetObservationFromSource(ctx context.Context, r *common.FileRange) (common.ObservationSchema, error) {
	// Codechange prompt is just added redundancy, the real change guardrail is in --tools below
	systemPrompt := `
YOU MUST NOT MAKE ANY CODECHANGES.
Prefer NO or MINIMAL exploration in the repository to answer the given question.
DO NOT MAKE ASSUMPTIONS, YOUR OBSERVATIONS MUST BE FACTUAL.
ACCEPT THE CODE AS IS, THERE IS NO NEED TO USE LSP QUERIES. Return ONLY raw JSON.
No markdown. No code fences.`

	prompt := fmt.Sprintf("Respond with the observation schema to describe the code range '%v' as read and provided to you below:\n```%v```", r.String(), string(r.GetBytes()))

	return QueryClaudeCode[common.ObservationSchema](ctx, systemPrompt, prompt, "medium")
}

type claudeBatchObservationInput struct {
	Sources     map[string]string `json:"sources"`
	Connections map[string]string `json:"connections"`
}

func (p *ClaudeCodeProvider) ResolveObservationsToGraph(ctx context.Context, g cog.ObservationGraph, root cog.COGNode) error {
	var innerErr error

	// Resolves relevant vertices and edges in a subgraph
	vertices := []cog.COGNode{}

	err := graph.BFS(g, root.GetHash(), func(h string) bool {
		if err := ctx.Err(); err != nil {
			innerErr = err
			return true
		}

		n, err := g.Vertex(h)
		if err != nil {
			innerErr = err
			return true
		}

		vertices = append(vertices, n)
		return false
	})

	if innerErr != nil {
		return innerErr
	}

	if err != nil {
		return err
	}

	edgeKeys, err := g.Edges()
	if err != nil {
		return err
	}

	edges := make([]graph.Edge[cog.COGNode], 0, len(edgeKeys))
	for _, ek := range edgeKeys {
		e, err := g.Edge(ek.Source, ek.Target)
		if err != nil {
			return err
		}
		edges = append(edges, e)
	}

	// Build a batch call
	in := claudeBatchObservationInput{
		Sources:     map[string]string{},
		Connections: map[string]string{},
	}

	for _, v := range vertices {
		in.Sources[v.GetHash()] = v.GetSource()
	}

	for _, e := range edges {
		ekey := fmt.Sprintf("edge:%v:%v", e.Source.GetHash(), e.Target.GetHash())
		in.Connections[ekey] = fmt.Sprintf("%v loads %v", e.Source.GetHash(), e.Target.GetHash())
	}

	// Codechange prompt is just added redundancy, the real change guardrail is in --tools below
	systemPrompt := `
	YOU MUST NOT MAKE ANY CODECHANGES.
	Prefer NO or MINIMAL exploration in the repository to answer the given question.
	DO NOT MAKE ASSUMPTIONS, YOUR OBSERVATIONS MUST BE FACTUAL.
	ACCEPT THE CODE AS IS, THERE IS NO NEED TO USE LSP QUERIES. Return ONLY raw JSON.
	No markdown. No code fences.
	YOU ARE REQUIRED TO KEEP A STRICT RELATIONSHIP BETWEEN INPUT KEYS AND OUTPUT DEFINITIONS`

	encin, err := json.Marshal(in)

	if err != nil {
		return fmt.Errorf("failed to serialize batch input for claude code %w", err)
	}

	prompt := fmt.Sprintf("Respond with the observation schema to described code below while keeping IDs intact:\n```%s```", encin)

	res, err := QueryClaudeCode[*common.BatchObservationSchema](ctx, systemPrompt, prompt, "high")

	if err != nil {
		return err
	}

	encres, err := json.Marshal(res)

	if err != nil {
		return err
	}

	gfile, _ := os.Create("./viz.gv")
	_ = draw.DOT(g, gfile)

	fmt.Printf("%s\n", encres)

	return nil
}

var _ cog.ObservationProvider = &ClaudeCodeProvider{}
