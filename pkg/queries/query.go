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
	return []PromptQuery{NewExplainBehaviorQuery()}
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

// Conformance checks
// banstructlit:ignore
var _ PromptQuery = ExplainBehaviorQuery{}
