package tests_test

import (
	"testing"

	"github.com/schahriar/captn/pkg/tui"
	"github.com/stretchr/testify/assert"
)

func TestClampScrollRegionsRewritesReset(t *testing.T) {
	out, pend := tui.ClampScrollRegions([]byte("\x1b7\x1b[r\x1b8"), 22)
	assert.Equal(t, "\x1b7\x1b[1;22r\x1b8", string(out))
	assert.Empty(t, pend)
}

func TestClampScrollRegionsClampsExplicitBottom(t *testing.T) {
	out, pend := tui.ClampScrollRegions([]byte("\x1b[5;24r"), 22)
	assert.Equal(t, "\x1b[5;22r", string(out))
	assert.Empty(t, pend)
}

func TestClampScrollRegionsKeepsConfinedRegion(t *testing.T) {
	out, pend := tui.ClampScrollRegions([]byte("\x1b[2;10r"), 22)
	assert.Equal(t, "\x1b[2;10r", string(out))
	assert.Empty(t, pend)
}

func TestClampScrollRegionsPassesOtherSequences(t *testing.T) {
	in := "plain \x1b[?2004h\x1b[38;2;10;20;30mcolor\x1b[0m\x1b[2J\x1b[24;1H\x1bM"
	out, pend := tui.ClampScrollRegions([]byte(in), 22)
	assert.Equal(t, in, string(out))
	assert.Empty(t, pend)
}

func TestClampScrollRegionsHoldsSplitSequence(t *testing.T) {
	out, pend := tui.ClampScrollRegions([]byte("abc\x1b[1;2"), 22)
	assert.Equal(t, "abc", string(out))
	assert.Equal(t, "\x1b[1;2", string(pend))

	out, pend = tui.ClampScrollRegions(append(pend, []byte("4r tail")...), 22)
	assert.Equal(t, "\x1b[1;22r tail", string(out))
	assert.Empty(t, pend)
}

func TestClampScrollRegionsHoldsTrailingEscape(t *testing.T) {
	out, pend := tui.ClampScrollRegions([]byte("tail\x1b"), 22)
	assert.Equal(t, "tail", string(out))
	assert.Equal(t, "\x1b", string(pend))
}

func TestClampScrollRegionsFlushesOversizedPartial(t *testing.T) {
	in := "\x1b[123456789012345678"
	out, pend := tui.ClampScrollRegions([]byte(in), 22)
	assert.Equal(t, in, string(out))
	assert.Empty(t, pend)
}

func TestClampScrollRegionsDropsResetOnTinyRegion(t *testing.T) {
	out, pend := tui.ClampScrollRegions([]byte("a\x1b[rb"), 1)
	assert.Equal(t, "ab", string(out))
	assert.Empty(t, pend)
}
