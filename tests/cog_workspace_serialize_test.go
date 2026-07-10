package tests_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/common"
	"github.com/stretchr/testify/assert"
)

func newTestAnchor(workspace string, path string, sl int, sc int, el int, ec int) *common.FileRange {
	src := common.NewSource(workspace, path, nil)
	return common.NewFileRange(
		src,
		common.NewFilePosition(src, sl, sc, 0),
		common.NewFilePosition(src, el, ec, 0),
	)
}

func TestWorkspaceMarshalUnmarshalRoundTrip(t *testing.T) {
	dir := t.TempDir()
	wspace := cog.NewWorkspace(dir)

	h1 := common.PrimaryHash("observation-one")
	h2 := common.PrimaryHash("observation-two")

	wspace.SetObservation(h1, cog.NewCOGObservation(
		common.NewObservationSchema("s0", "explains the parser entrypoint and how sources are loaded"),
		[]*common.FileRange{
			newTestAnchor(dir, "pkg/parser/parse.go", 3, 0, 10, 1),
			newTestAnchor(dir, "pkg/with-dash/and:colon.go", 0, 0, 2, 5),
		},
	))
	wspace.SetObservation(h2, cog.NewCOGObservation(
		common.NewObservationSchema("s1", "answer with spaces, = signs and : colons - even dashes"),
		nil,
	))

	h3 := common.PrimaryHash("observation-three")
	wspace.SetObservation(h3, cog.NewCOGObservation(
		common.NewObservationSchema("s2", "a multi-line answer\nwith a second line\n\nand a trailing newline\n"),
		[]*common.FileRange{newTestAnchor(dir, "cmd/main.go", 0, 0, 1, 1)},
	))

	b, err := wspace.Marshal()
	assert.NoError(t, err)

	decoded := cog.NewWorkspace(dir)
	assert.NoError(t, decoded.Unmarshal(b))

	assert.Len(t, decoded.ObservationCache, 3)

	o1 := decoded.ObservationCache[h1]
	assert.Equal(t, h1.String(), o1.Answer.ID)
	assert.Equal(t, "explains the parser entrypoint and how sources are loaded", o1.Answer.Answer)
	assert.Len(t, o1.Metadata.Anchors, 2)
	assert.Equal(t, "pkg/parser/parse.go:4:1-11:2", o1.Metadata.Anchors[0].String())
	assert.Equal(t, "pkg/with-dash/and:colon.go:1:1-3:6", o1.Metadata.Anchors[1].String())
	assert.Equal(t, dir, o1.Metadata.Anchors[0].Source.Workspace)

	o2 := decoded.ObservationCache[h2]
	assert.Equal(t, h2.String(), o2.Answer.ID)
	assert.Empty(t, o2.Metadata.Anchors)

	o3 := decoded.ObservationCache[h3]
	assert.Equal(t, "a multi-line answer\nwith a second line\n\nand a trailing newline\n", o3.Answer.Answer)
	assert.Len(t, o3.Metadata.Anchors, 1)
}

func TestWorkspaceUnmarshalRejectsTruncatedAnswer(t *testing.T) {
	wspace := cog.NewWorkspace(t.TempDir())
	wspace.SetObservation(common.PrimaryHash("x"), cog.NewCOGObservation(
		common.NewObservationSchema("s0", "an answer that will be cut short"),
		nil,
	))

	b, err := wspace.Marshal()
	assert.NoError(t, err)

	decoded := cog.NewWorkspace(t.TempDir())
	assert.Error(t, decoded.Unmarshal(b[:len(b)-10]))
}

func TestWorkspaceUnmarshalRejectsMalformedLines(t *testing.T) {
	for name, data := range map[string]string{
		"missing separator":   "not-a-valid-line",
		"bad hash":            "nothash 1:90 {}",
		"bad length header":   common.PrimaryHash("x").String() + " abc:90 {}",
		"truncated hex":       common.PrimaryHash("x").String() + " 5:90 {}",
		"hex not messagepack": common.PrimaryHash("x").String() + " 1:zz {}",
	} {
		wspace := cog.NewWorkspace(t.TempDir())
		assert.Error(t, wspace.Unmarshal([]byte(data)), name)
	}
}

func TestWorkspaceMarshalPreservesOrder(t *testing.T) {
	wspace := cog.NewWorkspace(t.TempDir())
	hashes := []common.HashType{
		common.PrimaryHash("zeta"),
		common.PrimaryHash("alpha"),
		common.PrimaryHash("mid"),
	}

	for i, h := range hashes {
		wspace.SetObservation(h, cog.NewCOGObservation(
			common.NewObservationSchema(h.String(), fmt.Sprintf("answer %d", i)),
			nil,
		))
	}

	b, err := wspace.Marshal()
	assert.NoError(t, err)

	loaded := cog.NewWorkspace(t.TempDir())
	assert.NoError(t, loaded.Unmarshal(b))

	// New observations are appended after the ones loaded from disk
	h4 := common.PrimaryHash("appended")
	loaded.SetObservation(h4, cog.NewCOGObservation(
		common.NewObservationSchema(h4.String(), "answer 3"),
		nil,
	))

	b2, err := loaded.Marshal()
	assert.NoError(t, err)
	assert.Equal(t, string(b), string(b2[:len(b)]))

	lines := strings.Split(strings.TrimSpace(string(b2)), "\n")
	assert.Len(t, lines, 4)

	for i, h := range append(hashes, h4) {
		assert.True(t, strings.HasPrefix(lines[i], h.String()+" "), "line %d should start with hash %v", i, h.String())
	}
}

func TestUnmarshalFileRange(t *testing.T) {
	fr, err := common.UnmarshalFileRange("/w", "cmd/main.go:12:5-40:1")
	assert.NoError(t, err)
	assert.Equal(t, "cmd/main.go", fr.Source.Path)
	assert.Equal(t, 11, fr.Start.Line)
	assert.Equal(t, 4, fr.Start.Column)
	assert.Equal(t, 39, fr.End.Line)
	assert.Equal(t, 0, fr.End.Column)
	assert.Equal(t, "cmd/main.go:12:5-40:1", fr.String())

	for _, invalid := range []string{"", "main.go", "main.go:1:1", "main.go:0:1-2:2", "main.go:a:1-2:2"} {
		_, err := common.UnmarshalFileRange("/w", invalid)
		assert.Error(t, err, invalid)
	}
}
