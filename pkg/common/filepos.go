package common

import "fmt"

type FilePosition struct {
	Source *Source

	Line         int
	Column       int
	BytePosition int
}

func (fp FilePosition) String() string {
	return fmt.Sprintf("%v:%v", fp.Line, fp.Column)
}

func NewFilePosition(src *Source, line int, col int, bp int) FilePosition {
	return FilePosition{
		Source:       src,
		Line:         line,
		Column:       col,
		BytePosition: bp,
	}
}

type FileRange struct {
	Source *Source
	Start  FilePosition
	End    FilePosition
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
