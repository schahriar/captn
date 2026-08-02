package tui

import (
	"bytes"
	"fmt"
	"strconv"
)

// A held-back partial sequence longer than this cannot be a DECSTBM and is
// flushed through unchanged.
const maxRegionSeqLen = 16

// ClampScrollRegions rewrites DECSTBM (ESC[<top>;<bottom>r) sequences in a raw
// terminal stream so they never extend past bottom. Child TUIs reset the
// scroll region during startup (ESC[r), which would otherwise wipe the region
// that keeps the overlay's reserved rows out of the child's reach. A sequence
// split across chunks is returned in pend and must be prepended to the next
// chunk.
func ClampScrollRegions(data []byte, bottom int) (out []byte, pend []byte) {
	for {
		i := bytes.IndexByte(data, 0x1b)
		if i < 0 {
			if out == nil {
				return data, nil
			}
			return append(out, data...), nil
		}

		out = append(out, data[:i]...)
		data = data[i:]

		j := 1
		if j < len(data) && data[j] == '[' {
			j++
			for j < len(data) && (data[j] == ';' || (data[j] >= '0' && data[j] <= '9')) {
				j++
			}
			if j < len(data) && data[j] == 'r' {
				out = append(out, clampRegionSeq(data[2:j], bottom)...)
				data = data[j+1:]
				continue
			}
		}

		if j >= len(data) {
			if len(data) <= maxRegionSeqLen {
				return out, append([]byte(nil), data...)
			}
			return append(out, data...), nil
		}

		out = append(out, data[:j]...)
		data = data[j:]
	}
}

func clampRegionSeq(params []byte, bottom int) []byte {
	if bottom < 2 {
		return nil
	}

	top, bot := 1, bottom
	parts := bytes.Split(params, []byte{';'})
	if v, err := strconv.Atoi(string(parts[0])); err == nil && v > 0 {
		top = v
	}
	if len(parts) > 1 {
		if v, err := strconv.Atoi(string(parts[1])); err == nil && v > 0 {
			bot = v
		}
	}

	if bot > bottom {
		bot = bottom
	}
	if top < 1 || top >= bot {
		top, bot = 1, bottom
	}

	return fmt.Appendf(nil, ansiRegionPairFmt, top, bot)
}
