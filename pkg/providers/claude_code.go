package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/dominikbraun/graph"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/queries"
)

type ClaudeCodeProvider struct{}

func NewClaudeCodeProvider() *ClaudeCodeProvider {
	return &ClaudeCodeProvider{}
}

func NewClaudeResult() *ClaudeResult {
	return &ClaudeResult{}
}

func NewclaudeBatchObservationInput() claudeBatchObservationInput {
	return claudeBatchObservationInput{
		Sources:     []claudeIdentifiedObservationInput{},
		Connections: []claudeIdentifiedObservationInput{},
	}
}

func NewclaudeIdentifiedObservationInput(id, description string) claudeIdentifiedObservationInput {
	return claudeIdentifiedObservationInput{
		ID:          id,
		Description: description,
	}
}

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
	sscma, err := scma.Marshal()

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
		"--json-schema", string(sscma),
	)

	out, err := cmd.CombinedOutput()

	if err != nil {
		return scma, fmt.Errorf("failed to run claude code with error %w", err)
	}

	res := NewClaudeResult()

	if err := json.Unmarshal(out, &res); err != nil {
		return scma, fmt.Errorf("unexpected response from claude with error %w \n Received: %s", err, out)
	}

	if err := json.Unmarshal([]byte(res.Result), &scma); err != nil {
		return scma, fmt.Errorf("structured output did not match expected schema with error %w \n Received: %s", err, res.Result)
	}

	return scma, nil
}

type claudeIdentifiedObservationInput struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

type claudeBatchObservationInput struct {
	Sources     []claudeIdentifiedObservationInput `json:"sources"`
	Connections []claudeIdentifiedObservationInput `json:"connections"`
}

func (p *ClaudeCodeProvider) Query(ctx context.Context, wspace *cog.Workspace, g *cog.RootedObservationGraph, q queries.PromptQuery) error {
	var innerErr error

	// Resolves relevant vertices and edges in a subgraph
	vertices := []cog.COGNode{}

	err := graph.BFS(g.Graph, g.Root.GetHash(), func(h common.HashType) bool {
		if err := ctx.Err(); err != nil {
			innerErr = err
			return true
		}

		n, err := g.Graph.Vertex(h)
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

	edgeKeys, err := g.Graph.Edges()
	if err != nil {
		return err
	}

	edges := make([]graph.Edge[cog.COGNode], 0, len(edgeKeys))
	for _, ek := range edgeKeys {
		e, err := g.Graph.Edge(ek.Source, ek.Target)
		if err != nil {
			return err
		}
		edges = append(edges, e)
	}

	// Build a batch call
	in := NewclaudeBatchObservationInput()

	// Short, sequential string IDs for the LLM round-trip; raw uint32 hashes are
	// fragile to copy verbatim and LLMs frequently drift digits.
	verteximap := map[string]struct {
		Node common.HashType
		Key  common.HashType
	}{}
	edgeimap := map[string]struct {
		Source common.HashType
		Target common.HashType
		Hash   common.HashType
	}{}

	wspace.Mux.Lock()

	for _, v := range vertices {
		k := v.GetHash()
		key := queries.ObservationKey(k, q)
		// A cache hit still has to be written onto this run's graph; the cache is
		// only consulted to skip the LLM round-trip, the attribute it carries is
		// what the explanation renders from.
		if cached, ok := wspace.ObservationCache[key]; ok {
			g.Graph.SetVertexAttribute(k, "observation-answer", cached.Answer.Answer)
			continue
		}

		sid := fmt.Sprintf("s%d", len(in.Sources))
		verteximap[sid] = struct {
			Node common.HashType
			Key  common.HashType
		}{
			Node: k,
			Key:  key,
		}
		in.Sources = append(in.Sources, NewclaudeIdentifiedObservationInput(sid, v.GetStringSource()))
	}

	for _, e := range edges {
		ekey := queries.ObservationKey(common.PrimaryHash(fmt.Sprintf("edge:%v:%v", e.Source.GetHash(), e.Target.GetHash())), q)

		if cached, ok := wspace.ObservationCache[ekey]; ok {
			g.Graph.SetEdgeAttribute(e.Source.GetHash(), e.Target.GetHash(), "observation-answer", cached.Answer.Answer)
			continue
		}

		sid := fmt.Sprintf("c%d", len(in.Connections))
		edgeimap[sid] = struct {
			Source common.HashType
			Target common.HashType
			Hash   common.HashType
		}{
			Source: e.Source.GetHash(),
			Target: e.Target.GetHash(),
			Hash:   ekey,
		}
		in.Connections = append(in.Connections, NewclaudeIdentifiedObservationInput(sid, fmt.Sprintf("%v loads %v", e.Source.GetHash(), e.Target.GetHash())))
	}

	wspace.Mux.Unlock()

	if len(in.Connections)+len(in.Sources) == 0 {
		// Everything can be read from cache
		return nil
	}

	// Codechange prompt is just added redundancy, the real change guardrail is in --tools below
	systemPrompt := `
	YOU MUST NOT MAKE ANY CODECHANGES.
	Prefer NO or MINIMAL exploration in the repository to answer the given question.
	DO NOT MAKE ASSUMPTIONS, YOUR OBSERVATIONS MUST BE FACTUAL.
	ACCEPT THE GIVEN CODE AS IS, THERE IS NO NEED TO USE LSP QUERIES. Return ONLY raw JSON.
	No markdown. No code fences. DO NOT echo the input back.
	You will be given an input object with "sources" and "connections" arrays.
	- For every entry in "sources" you MUST emit exactly one entry in the output "observations" array.
	- For every entry in "connections" you MUST emit exactly one entry in the output "connectionObservations" array.
	- Your outputs must represent a reasoning trace as a teacher model for a student model to understand the code and its connections.
	Each output entry has two fields: "id" (copied verbatim from the matching input entry) and "answer"
	DO NOT REPEAT CODE PROVIDED TO YOU UNLESS YOU ARE PROVIDING AN EXAMPLE SNIPPET FOR EXPLANATIONS / OBSERVATIONS
	(a concise, factual description of what the code does, skip niceties).
	Each answer MUST stand on its own. NEVER reference another entry by its "s"/"c" id (e.g. "same as s0", "identical to s1") — those ids are internal and not shown to the reader. Describe the code directly even if another entry is identical.
	Output must be valid JSON matching the supplied json-schema.`

	encin, err := json.Marshal(in)

	if err != nil {
		return fmt.Errorf("failed to serialize batch input for claude code %w", err)
	}

	prompt := fmt.Sprintf(`%v

Input shape: {"sources":[{"id","description"}],"connections":[{"id","description"}]}
Output shape: {"observations":[{"id","answer"}],"connectionObservations":[{"id","answer"}]}
Input:
%s`, q.GetPrompt(), encin)

	res, err := QueryClaudeCode[*common.BatchObservationSchema](ctx, systemPrompt, prompt, "high")

	if err != nil {
		return err
	}

	wspace.Mux.Lock()
	defer wspace.Mux.Unlock()

	var responseErr error

	for _, o := range res.Observations {
		vdef, ok := verteximap[o.ID]
		if !ok {
			if responseErr == nil {
				responseErr = fmt.Errorf("claude code returned observation for unknown source id %q", o.ID)
			}
			continue
		}

		o.ID = vdef.Key.String()

		vn, err := g.Graph.Vertex(vdef.Node)

		if err != nil {
			if responseErr == nil {
				responseErr = fmt.Errorf("claude code returned observation for unknown source id %q with error %w", o.ID, err)
			}
			continue
		}

		anchors := []*common.FileRange{
			vn.GetFileRange(),
		}

		ans := cog.NewCOGObservation(o, anchors)

		g.Graph.SetVertexAttribute(vdef.Node, "observation-answer", o.Answer)
		wspace.SetObservationLocked(vdef.Key, ans)
	}

	for _, e := range res.ConnectionObservations {
		edef, ok := edgeimap[e.ID]
		if !ok {
			if responseErr == nil {
				responseErr = fmt.Errorf("claude code returned observation for unknown connection id %q", e.ID)
			}
			continue
		}

		e.ID = edef.Hash.String()

		sn, err := g.Graph.Vertex(edef.Source)

		if err != nil {
			if responseErr == nil {
				responseErr = fmt.Errorf("claude code returned observation for unknown connection id %q with error %w", e.ID, err)
			}
			continue
		}

		tn, err := g.Graph.Vertex(edef.Target)

		if err != nil {
			if responseErr == nil {
				responseErr = fmt.Errorf("claude code returned observation for unknown connection id %q with error %w", e.ID, err)
			}
			continue
		}

		anchors := []*common.FileRange{
			sn.GetFileRange(),
			tn.GetFileRange(),
		}

		answer := cog.NewCOGObservation(e, anchors)

		g.Graph.SetEdgeAttribute(edef.Source, edef.Target, "observation-answer", e.Answer)
		wspace.SetObservationLocked(edef.Hash, answer)
	}

	return responseErr
}

var _ cog.ObservationProvider = (*ClaudeCodeProvider)(nil)
