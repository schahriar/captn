package queries

import (
	"fmt"

	"github.com/schahriar/captn/pkg/common"
)

type PromptQuery interface {
	GetID() string
	GetHash() string
	GetPrompt() string
	GetRoutingDescription() string
	GetDisplayHints(provider string) []string
}

// defaultPromptIdentifierProvider hashes the query ID alongside the prompt so
// observations cached under one query can never be served for another, even if
// two queries share prompt text.
func defaultPromptIdentifierProvider(id, prompt string) string {
	return common.HashMany(id, prompt).String()
}

// ObservationKey derives the cache key for a (node, prompt) pair. HashMany is
// position-dependent, so node and prompt occupy separate slots and distinct
// pairs cannot collide by commutation.
func ObservationKey(node common.HashType, q PromptQuery) common.HashType {
	return common.HashMany(node.String(), q.GetHash())
}

// Supported lists the queries that can be selected by ID.
func Supported() []PromptQuery {
	return []PromptQuery{
		NewExplainBehaviorQuery(),
		NewExplainContractQuery(),
		NewListSideEffectsQuery(),
	}
}

func SupportedIDs() []string {
	qs := Supported()
	ids := make([]string, 0, len(qs))

	for _, q := range qs {
		ids = append(ids, q.GetID())
	}

	return ids
}

func ByID(id string) (PromptQuery, bool) {
	for _, q := range Supported() {
		if q.GetID() == id {
			return q, true
		}
	}

	return nil, false
}

type ExplainBehaviorQuery struct{}

func (q ExplainBehaviorQuery) GetID() string {
	return "explain_behavior"
}

func (q ExplainBehaviorQuery) GetHash() string {
	return defaultPromptIdentifierProvider(q.GetID(), q.GetPrompt())
}

func (q ExplainBehaviorQuery) GetRoutingDescription() string {
	return "explains the behavior of the given code snippet"
}

func (q ExplainBehaviorQuery) GetPrompt() string {
	return "explain the behavior of the given code snippet in concise reasoning format"
}

func (q ExplainBehaviorQuery) GetDisplayHints(provider string) []string {
	return []string{
		fmt.Sprintf("reasoning with %s", provider),
		"reading the code",
		"reasoning about behavior",
		"tracing the logic",
		"providing an explanation",
		fmt.Sprintf("reasoning with %s", provider),
	}
}

func NewExplainBehaviorQuery() ExplainBehaviorQuery {
	return ExplainBehaviorQuery{}
}

type ExplainContractQuery struct{}

func (q ExplainContractQuery) GetID() string {
	return "explain_contract"
}

func (q ExplainContractQuery) GetHash() string {
	return defaultPromptIdentifierProvider(q.GetID(), q.GetPrompt())
}

func (q ExplainContractQuery) GetRoutingDescription() string {
	return "describes the inputs, outputs, and error semantics the code guarantees to its callers"
}

func (q ExplainContractQuery) GetPrompt() string {
	return "describe the contract the given code guarantees to its callers: expected inputs, outputs, error semantics (returned, wrapped, swallowed, sentinel values), and behavior on nil, empty, or zero inputs"
}

func (q ExplainContractQuery) GetDisplayHints(provider string) []string {
	return []string{
		fmt.Sprintf("reasoning with %s", provider),
		"reading the code",
		"identifying inputs and outputs",
		"tracing error semantics",
		"writing the contract",
		fmt.Sprintf("reasoning with %s", provider),
	}
}

func NewExplainContractQuery() ExplainContractQuery {
	return ExplainContractQuery{}
}

type ListSideEffectsQuery struct{}

func (q ListSideEffectsQuery) GetID() string {
	return "list_side_effects"
}

func (q ListSideEffectsQuery) GetHash() string {
	return defaultPromptIdentifierProvider(q.GetID(), q.GetPrompt())
}

func (q ListSideEffectsQuery) GetRoutingDescription() string {
	return "lists the state the code mutates and the I/O it performs, or states that it is pure"
}

func (q ListSideEffectsQuery) GetPrompt() string {
	return "list the side effects of the given code: state it mutates (receivers, arguments, globals, package-level caches), I/O it performs (files, network, subprocesses, environment), and any goroutines, channels, or locks it uses; if it has no side effects, state that it is pure"
}

func (q ListSideEffectsQuery) GetDisplayHints(provider string) []string {
	return []string{
		fmt.Sprintf("reasoning with %s", provider),
		"reading the code",
		"tracking mutations",
		"tracing I/O",
		"listing side effects",
		fmt.Sprintf("reasoning with %s", provider),
	}
}

func NewListSideEffectsQuery() ListSideEffectsQuery {
	return ListSideEffectsQuery{}
}

// Conformance checks
// banstructlit:ignore
var _ PromptQuery = ExplainBehaviorQuery{}

// banstructlit:ignore
var _ PromptQuery = ExplainContractQuery{}

// banstructlit:ignore
var _ PromptQuery = ListSideEffectsQuery{}
