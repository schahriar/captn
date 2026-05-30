package lsp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func decodeDefinitionResult(raw json.RawMessage) ([]Location, error) {
	raw = bytes.TrimSpace(raw)

	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}

	if raw[0] == '{' {
		var loc Location
		if err := json.Unmarshal(raw, &loc); err == nil && loc.URI != "" {
			return []Location{loc}, nil
		}

		var link LocationLink
		if err := json.Unmarshal(raw, &link); err != nil {
			return nil, err
		}

		if link.TargetURI == "" {
			return nil, nil
		}

		return []Location{
			{
				URI:   link.TargetURI,
				Range: link.TargetSelectionRange,
			},
		}, nil
	}

	if raw[0] != '[' {
		return nil, fmt.Errorf("unexpected definition response: %s", string(raw))
	}

	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}

	locations := make([]Location, 0, len(values))

	for _, value := range values {
		var loc Location
		if err := json.Unmarshal(value, &loc); err == nil && loc.URI != "" {
			locations = append(locations, loc)
			continue
		}

		var link LocationLink
		if err := json.Unmarshal(value, &link); err != nil {
			return nil, err
		}

		if link.TargetURI == "" {
			continue
		}

		locations = append(locations, Location{
			URI:   link.TargetURI,
			Range: link.TargetSelectionRange,
		})
	}

	return locations, nil
}

func byteOffsetForUTF16Column(s string, column int) (int, error) {
	if column < 0 {
		return 0, fmt.Errorf("column cannot be negative")
	}

	current := 0

	for byteIndex, ch := range s {
		if current == column {
			return byteIndex, nil
		}

		width := utf16RuneWidth(ch)
		if current+width > column {
			return 0, fmt.Errorf("column lands inside utf-16 surrogate pair")
		}

		current += width
	}

	if current == column {
		return len(s), nil
	}

	return 0, fmt.Errorf("column out of bounds")
}

func utf16ColumnAtByteOffset(s string, byteOffset int) (int, error) {
	if byteOffset < 0 || byteOffset > len(s) {
		return 0, fmt.Errorf("byte offset out of bounds")
	}

	column := 0

	for byteIndex, ch := range s {
		if byteIndex == byteOffset {
			return column, nil
		}

		if byteIndex > byteOffset {
			return 0, fmt.Errorf("byte offset does not align with rune boundary")
		}

		column += utf16RuneWidth(ch)
	}

	if byteOffset == len(s) {
		return column, nil
	}

	return 0, fmt.Errorf("byte offset does not align with rune boundary")
}

func utf16Len(s string) int {
	n := 0

	for _, ch := range s {
		n += utf16RuneWidth(ch)
	}

	return n
}

func utf16RuneWidth(ch rune) int {
	if ch >= 0x10000 {
		return 2
	}

	return 1
}

func isIgnoredSelectionRune(ch rune) bool {
	switch ch {
	case '"', '`', '\'', ' ', '\t', '\r', '\n':
		return true
	default:
		return false
	}
}

func readFramedMessage(r *bufio.Reader) ([]byte, error) {
	contentLength := -1

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}

		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		if strings.EqualFold(name, "Content-Length") {
			_, err := fmt.Sscanf(strings.TrimSpace(value), "%d", &contentLength)
			if err != nil {
				return nil, err
			}
		}
	}

	if contentLength < 0 {
		return nil, fmt.Errorf("missing Content-Length")
	}

	body := make([]byte, contentLength)
	_, err := io.ReadFull(r, body)
	return body, err
}

func rangesOverlap(a Range, b Range) bool {
	if positionBeforeOrEqual(a.End, b.Start) {
		return false
	}

	if positionBeforeOrEqual(b.End, a.Start) {
		return false
	}

	return true
}

func positionBeforeOrEqual(a Position, b Position) bool {
	if a.Line < b.Line {
		return true
	}

	if a.Line > b.Line {
		return false
	}

	return a.Character <= b.Character
}
