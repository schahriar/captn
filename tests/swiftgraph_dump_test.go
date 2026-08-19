package tests_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/schahriar/captn/pkg/cog"
	"github.com/stretchr/testify/assert"
)

type strSink struct{ s string }

func (w *strSink) Write(p []byte) (int, error) { w.s += string(p); return len(p), nil }

// TestSwiftGraphDump prints the observation graph for a Swift search so node
// colours and shape can be eyeballed in tools/viz. Opt-in: it is a debug tool,
// not an assertion. CAPTN_PROBE=1 go test -run GraphDump ./tests
func TestSwiftGraphDump(t *testing.T) {
	if os.Getenv("CAPTN_PROBE") == "" {
		t.Skip("set CAPTN_PROBE=1 to dump the observation graph")
	}

	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}
	wspace := cog.NewWorkspace(cwd)
	og, _, err := wspace.SearchSnippet(t.Context(),
		"./fixtures/swift/multidep/Sources/App/main.swift",
		"print(getExampleText().uppercased())")
	if !assert.NoError(t, err) {
		return
	}
	sb := &strSink{}
	assert.NoError(t, og.WriteDOT(sb))
	fmt.Println(sb.s)
}
