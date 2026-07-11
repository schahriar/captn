package queries

import (
	"fmt"

	"github.com/schahriar/captn/pkg/common"
)

type PromptQuery interface {
	GetIdentifier() string
	GetPrompt() string
	GetDisplayHints(provider string) []string
}

func defaultPromptIdentifierProvider(d string) string {
	return common.PrimaryHash(d).String()
}

// ObservationKey derives the cache key for a (node, prompt) pair. HashMany is
// position-dependent, so node and prompt occupy separate slots and distinct
// pairs cannot collide by commutation.
func ObservationKey(node common.HashType, q PromptQuery) common.HashType {
	return common.HashMany(node.String(), q.GetIdentifier())
}

type ExplainBehaviorQuery struct{}

func (q ExplainBehaviorQuery) GetIdentifier() string {
	return defaultPromptIdentifierProvider(q.GetPrompt())
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
