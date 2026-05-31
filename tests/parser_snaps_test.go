package tests_test

import (
	"testing"

	"github.com/gkampitakis/go-snaps/snaps"
)

func TestParserSimpleFuncParse(t *testing.T) {
	snaps.MatchYAML(t, prog.AST().Debug())
}
