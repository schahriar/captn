package tests_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/schahriar/captn/pkg/cog"
	"github.com/stretchr/testify/assert"
)

// TestPHPGraphDump prints the observation graph for a PHP search so node
// colours and shape can be eyeballed in tools/viz. Opt-in: it is a debug
// tool, not an assertion. CAPTN_PROBE=1 go test -run GraphDump ./tests
func TestPHPGraphDump(t *testing.T) {
	if os.Getenv("CAPTN_PROBE") == "" {
		t.Skip("set CAPTN_PROBE=1 to dump the observation graph")
	}

	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	wspace := cog.NewWorkspace(cwd)

	// intelephense indexes in the background
	var og *cog.ObservationGraph

	for attempt := 0; attempt < 10; attempt++ {
		g, _, err := wspace.SearchSnippet(t.Context(), "./fixtures/php/multidep/main.php",
			"$now = new DateTime();\n    return FixtureDep1::exampleText($now);")

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
