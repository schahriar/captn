package tests_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/schahriar/captn/pkg/cog"
	"github.com/stretchr/testify/assert"
)

// TestTypescriptGraphDump prints the observation graph for a TypeScript search
// so node colours and shape can be eyeballed in tools/viz. Opt-in: it is a
// debug tool, not an assertion. CAPTN_PROBE=1 go test -run GraphDump ./tests
func TestTypescriptGraphDump(t *testing.T) {
	if os.Getenv("CAPTN_PROBE") == "" {
		t.Skip("set CAPTN_PROBE=1 to dump the observation graph")
	}

	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	wspace := cog.NewWorkspace(cwd)

	pf, err := wspace.LoadFile(t.Context(), "./fixtures/typescript/multidep/main.ts")
	if !assert.NoError(t, err) {
		return
	}

	if tsAwaitDependencies(t, pf, 2) == nil {
		return
	}

	og, _, err := wspace.SearchSnippet(t.Context(),
		"./fixtures/typescript/multidep/main.ts",
		`readFileSync(fixtureDep1(), "utf8")`)
	if !assert.NoError(t, err) {
		return
	}

	sb := &strSink{}
	assert.NoError(t, og.WriteDOT(sb))
	fmt.Println(sb.s)
}
