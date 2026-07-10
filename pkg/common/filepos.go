package common

import (
	"fmt"
	"strconv"
	"strings"
)

type FilePosition struct {
	Source *Source

	Line         int
	Column       int
	BytePosition int
}

func (fp FilePosition) String() string {
	return fmt.Sprintf("%v:%v", fp.Line+1, fp.Column+1)
}

func NewFilePosition(src *Source, line int, col int, bp int) FilePosition {
	return FilePosition{
		Source:       src,
		Line:         line,
		Column:       col,
		BytePosition: bp,
	}
}

func FirstPositionOfSource(src *Source) FilePosition {
	return NewFilePosition(src, 0, 0, 0)
}

func LastPositionOfSource(src *Source) FilePosition {
	line, col, bp := LastLineColumnBytePosition(src)
	return NewFilePosition(src, line, col, bp)
}

func NewFileRangeAutoBytePosition(src *Source, sl int, sc int, el int, ec int) (*FileRange, error) {
	startBP, err := src.BytePositionForLineColumn(sl, sc)
	if err != nil {
		return nil, err
	}

	endBP, err := src.BytePositionForLineColumn(el, ec)
	if err != nil {
		return nil, err
	}

	start := NewFilePosition(src, sl, sc, startBP)
	end := NewFilePosition(src, el, ec, endBP)

	return NewFileRange(src, start, end), nil
}

func LastLineColumnBytePosition(src *Source) (int, int, int) {
	if src == nil {
		return 0, 0, 0
	}

	line := 0
	lineStart := 0

	for i, b := range src.Buffer {
		if b == '\n' {
			line++
			lineStart = i + 1
		}
	}

	return line, len(src.Buffer) - lineStart, len(src.Buffer)
}

func (fp FilePosition) Before(other FilePosition) bool {
	return fp.BytePosition < other.BytePosition
}

func (fp FilePosition) After(other FilePosition) bool {
	return fp.BytePosition > other.BytePosition
}

func (fp FilePosition) InsideRange(fr FileRange) bool {
	return fr.Contains(fp)
}

func CompareFilePosition(a FilePosition, b FilePosition) int {
	switch {
	case a.After(b):
		return 1
	case a.Before(b):
		return -1
	default:
		return 0
	}
}

type FileRange struct {
	Source *Source
	Start  FilePosition
	End    FilePosition
}

func (fr FileRange) GetBytes() []byte {
	return fr.Source.Buffer[fr.Start.BytePosition:fr.End.BytePosition]
}

func (fr FileRange) Before(cr FileRange) bool {
	return fr.Start.BytePosition < cr.Start.BytePosition
}

func (fr FileRange) After(cr FileRange) bool {
	return fr.Start.BytePosition > cr.Start.BytePosition
}

func (fr FileRange) Contains(fp FilePosition) bool {
	return fp.BytePosition >= fr.Start.BytePosition && fp.BytePosition < fr.End.BytePosition
}

func (fr FileRange) ContainedBy(outer FileRange) bool {
	return fr.Start.BytePosition >= outer.Start.BytePosition &&
		fr.End.BytePosition <= outer.End.BytePosition
}

func CompareFileRange(a FileRange, b FileRange) int {
	switch {
	case a.After(b):
		return 1
	case a.Before(b):
		return -1
	default:
		return 0
	}
}

func (fr FileRange) String() string {
	return fmt.Sprintf("%v:%v-%v", fr.Source.Path, fr.Start.String(), fr.End.String())
}

func (fr FileRange) GetByteRange() [2]int {
	return [2]int{fr.Start.BytePosition, fr.End.BytePosition}
}

// UnmarshalFileRange parses the format produced by FileRange.String().
// The path may itself contain ':' and '-' so positions are parsed from the right.
// Byte positions are not part of the string form and are left at 0.
func UnmarshalFileRange(workspace string, s string) (*FileRange, error) {
	parsePosition := func(v string) (int, int, error) {
		lineStr, colStr, ok := strings.Cut(v, ":")

		if !ok {
			return 0, 0, fmt.Errorf("invalid file position %q", v)
		}

		line, err := strconv.Atoi(lineStr)

		if err != nil {
			return 0, 0, fmt.Errorf("invalid line in file position %q", v)
		}

		col, err := strconv.Atoi(colStr)

		if err != nil {
			return 0, 0, fmt.Errorf("invalid column in file position %q", v)
		}

		if line < 1 || col < 1 {
			return 0, 0, fmt.Errorf("file position %q is out of range", v)
		}

		return line - 1, col - 1, nil
	}

	sep := strings.LastIndex(s, "-")

	if sep < 0 {
		return nil, fmt.Errorf("invalid file range %q", s)
	}

	endLine, endCol, err := parsePosition(s[sep+1:])

	if err != nil {
		return nil, err
	}

	left := s[:sep]
	colSep := strings.LastIndex(left, ":")

	if colSep < 0 {
		return nil, fmt.Errorf("invalid file range %q", s)
	}

	lineSep := strings.LastIndex(left[:colSep], ":")

	if lineSep < 0 {
		return nil, fmt.Errorf("invalid file range %q", s)
	}

	startLine, startCol, err := parsePosition(left[lineSep+1:])

	if err != nil {
		return nil, err
	}

	src := NewSource(workspace, left[:lineSep], nil)

	return NewFileRange(
		src,
		NewFilePosition(src, startLine, startCol, 0),
		NewFilePosition(src, endLine, endCol, 0),
	), nil
}

func NewFileRange(src *Source, start FilePosition, end FilePosition) *FileRange {
	return &FileRange{
		Source: src,
		Start:  start,
		End:    end,
	}
}
