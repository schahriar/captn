package tests_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/schahriar/captn/pkg/cog"
	"github.com/stretchr/testify/assert"
)

// TestCGraphDump prints the observation graph for a C search so node colours
// and shape can be eyeballed in tools/viz. Opt-in: it is a debug tool, not an
// assertion. CAPTN_PROBE=1 go test -run GraphDump ./tests
func TestCGraphDump(t *testing.T) {
	if os.Getenv("CAPTN_PROBE") == "" {
		t.Skip("set CAPTN_PROBE=1 to dump the observation graph")
	}

	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	wspace := cog.NewWorkspace(cwd)

	og, _, err := wspace.SearchSnippet(t.Context(), "./fixtures/c/multidep/main.c",
		"Widget w = widget_make(\"card\");\n\treturn (int)strlen(w.label) + widget_area(&w);")

	if !assert.NoError(t, err) {
		return
	}

	sb := &strSink{}
	assert.NoError(t, og.WriteDOT(sb))
	fmt.Println(sb.s)
}

func TestCPPGraphDump(t *testing.T) {
	if os.Getenv("CAPTN_PROBE") == "" {
		t.Skip("set CAPTN_PROBE=1 to dump the observation graph")
	}

	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	wspace := cog.NewWorkspace(cwd)

	og, _, err := wspace.SearchSnippet(t.Context(), "./fixtures/cpp/multidep/main.cpp",
		"gadgets::Gadget g(\"card\");\n\tstd::string label = g.describe();")

	if !assert.NoError(t, err) {
		return
	}

	sb := &strSink{}
	assert.NoError(t, og.WriteDOT(sb))
	fmt.Println(sb.s)
}
