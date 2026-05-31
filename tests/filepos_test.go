package tests_test

import (
	"testing"

	"github.com/schahriar/captn/pkg/common"
	"github.com/stretchr/testify/assert"
)

func TestFilePositionString(t *testing.T) {
	src := &common.Source{Path: "file.go"}

	fp := common.NewFilePosition(src, 0, 0, 0)
	assert.Equal(t, "1:1", fp.String())

	fp = common.NewFilePosition(src, 4, 9, 100)
	assert.Equal(t, "5:10", fp.String())
}

func TestFilePositionFields(t *testing.T) {
	src := &common.Source{Path: "file.go"}

	fp := common.NewFilePosition(src, 2, 3, 42)
	assert.Equal(t, src, fp.Source)
	assert.Equal(t, 2, fp.Line)
	assert.Equal(t, 3, fp.Column)
	assert.Equal(t, 42, fp.BytePosition)
}

func TestFileRangeString(t *testing.T) {
	src := &common.Source{Path: "main.go"}
	start := common.NewFilePosition(src, 0, 0, 0)
	end := common.NewFilePosition(src, 2, 5, 50)

	fr := common.NewFileRange(src, start, end)
	assert.Equal(t, "main.go:1:1-3:6", fr.String())
}

func TestFileRangeGetByteRange(t *testing.T) {
	src := &common.Source{Path: "main.go"}
	start := common.NewFilePosition(src, 0, 0, 10)
	end := common.NewFilePosition(src, 1, 3, 30)

	fr := common.NewFileRange(src, start, end)
	assert.Equal(t, [2]int{10, 30}, fr.GetByteRange())
}

func TestFileRangeFields(t *testing.T) {
	src := &common.Source{Path: "pkg/foo.go"}
	start := common.NewFilePosition(src, 0, 0, 0)
	end := common.NewFilePosition(src, 0, 5, 5)

	fr := common.NewFileRange(src, start, end)
	assert.Equal(t, src, fr.Source)
	assert.Equal(t, start, fr.Start)
	assert.Equal(t, end, fr.End)
}
