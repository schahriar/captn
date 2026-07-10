package queries

import "github.com/schahriar/captn/pkg/common"

type PromptQuery interface {
	GetIdentifier() string
	GetPrompt() string
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

func NewExplainBehaviorQuery() ExplainBehaviorQuery {
	return ExplainBehaviorQuery{}
}

// Conformance checks
// banstructlit:ignore
var _ PromptQuery = ExplainBehaviorQuery{}
