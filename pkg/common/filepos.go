package common

import "fmt"

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

func NewFileRangeAutoBytePosition(src *Source, sl int, sc int, el int, ec int) (FileRange, error) {
	startBP, err := src.BytePositionForLineColumn(sl, sc)
	if err != nil {
		return FileRange{}, err
	}

	endBP, err := src.BytePositionForLineColumn(el, ec)
	if err != nil {
		return FileRange{}, err
	}

	start := NewFilePosition(src, sl, sc, startBP)
	end := NewFilePosition(src, el, ec, endBP)

	return NewFileRange(src, start, end), nil
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

func NewFileRange(src *Source, start FilePosition, end FilePosition) FileRange {
	return FileRange{
		Source: src,
		Start:  start,
		End:    end,
	}
}
