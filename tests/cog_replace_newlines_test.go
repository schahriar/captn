package tests_test

import (
	"testing"

	"github.com/schahriar/captn/pkg/cog"
	"github.com/stretchr/testify/assert"
)

func TestReplaceNewlines(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"empty":              {"", ""},
		"no newline":         {"a single line answer", "a single line answer"},
		"lf":                 {"first\nsecond", "first - second"},
		"crlf counts once":   {"first\r\nsecond", "first - second"},
		"lone cr":            {"first\rsecond", "first - second"},
		"consecutive":        {"first\n\nsecond", "first -  - second"},
		"leading":            {"\nfirst", " - first"},
		"trailing":           {"first\n", "first - "},
		"mixed line endings": {"a\r\nb\nc\rd", "a - b - c - d"},
		"only newlines":      {"\n\r\n\r", " -  -  - "},
	} {
		assert.Equal(t, tc.want, string(cog.ReplaceNewlines([]byte(tc.in), []byte(" - "))), name)
	}
}

func TestReplaceNewlinesDoesNotMutateInput(t *testing.T) {
	in := []byte("first\nsecond")
	cog.ReplaceNewlines(in, []byte(" - "))
	assert.Equal(t, "first\nsecond", string(in))
}
