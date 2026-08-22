package tests_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/schahriar/captn/pkg/cog"
	"github.com/stretchr/testify/assert"
)

// TestRubyGraphDump prints the observation graph for a Ruby search so node
// colours and shape can be eyeballed in tools/viz. Opt-in: it is a debug
// tool, not an assertion. CAPTN_PROBE=1 go test -run GraphDump ./tests
func TestRubyGraphDump(t *testing.T) {
	if os.Getenv("CAPTN_PROBE") == "" {
		t.Skip("set CAPTN_PROBE=1 to dump the observation graph")
	}

	wspace := cog.NewWorkspace(rubyWorkspace(t, "multidep"))

	// ruby-lsp indexes the workspace in the background after initialize
	var og *cog.ObservationGraph

	for attempt := 0; attempt < 10; attempt++ {
		g, _, err := wspace.SearchSnippet(t.Context(), "main.rb", "puts JSON.generate(Dep1.example_text)")

		if !assert.NoError(t, err) {
			return
		}

		og = g

		adj, err := og.Graph.AdjacencyMap()
		if !assert.NoError(t, err) {
			return
		}

		if len(adj) >= 3 {
			break
		}

		time.Sleep(2 * time.Second)
	}

	sb := &strSink{}
	assert.NoError(t, og.WriteDOT(sb))
	fmt.Println(sb.s)
}
