package tui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

type RGB struct{ r, g, b int }

func NewRGB(r, g, b int) RGB {
	return RGB{r: r, g: g, b: b}
}

var (
	shimmerBase = NewRGB(150, 132, 226)
	shimmerPeak = NewRGB(232, 226, 255)
)

const (
	shimmerBand = 0.16
	shimmerGap  = 0.65
	shimmerStep = 0.045
)

// Shimmer markers delimit text that renderShimmer animates. They are
// zero-width control bytes that never reach the terminal.
const (
	shimmerOn  = "\x01"
	shimmerOff = "\x02"
	shimmerSep = "\x1f"
)

// Shimmer marks s for the animated highlight sweep. It composes with other
// styling, e.g. Shimmer(Bold("captn") + "ing").
func Shimmer(s string) string {
	return ShimmerColor(shimmerBase, shimmerPeak)(s)
}

func ShimmerColor(base, peak RGB) func(string) string {
	header := fmt.Sprintf("%d;%d;%d;%d;%d;%d%s",
		base.r, base.g, base.b, peak.r, peak.g, peak.b, shimmerSep)
	return func(s string) string {
		return shimmerOn + header + s + shimmerOff
	}
}

type shimmerPart struct {
	text   string
	marked bool
	base   RGB
	peak   RGB
}

func splitShimmer(s string) []shimmerPart {
	var parts []shimmerPart
	var cur shimmerPart
	start := 0
	flush := func(end int) {
		if end > start {
			cur.text = s[start:end]
			parts = append(parts, cur)
		}
	}
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case shimmerOn[0]:
			flush(i)
			base, peak, n := parseShimmerHeader(s[i+1:])
			cur.marked = true
			cur.base = base
			cur.peak = peak
			i += n
			start = i + 1
		case shimmerOff[0]:
			flush(i)
			cur.marked = false
			start = i + 1
		}
	}
	flush(len(s))
	return parts
}

func parseShimmerHeader(s string) (RGB, RGB, int) {
	end := strings.IndexByte(s, shimmerSep[0])
	if end < 0 {
		return shimmerBase, shimmerPeak, 0
	}
	fields := strings.Split(s[:end], ";")
	if len(fields) != 6 {
		return shimmerBase, shimmerPeak, end + 1
	}
	var v [6]int
	for i, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil {
			return shimmerBase, shimmerPeak, end + 1
		}
		v[i] = n
	}
	return NewRGB(v[0], v[1], v[2]), NewRGB(v[3], v[4], v[5]), end + 1
}

// renderShimmer resolves Shimmer markers in line. Each marked segment gets
// its own self-contained highlight sweep, synced to the shared phase.
// Unmarked text and ANSI sequences inside marked text pass through untouched.
func renderShimmer(line string, phase float64) string {
	center := phase*(1+2*shimmerBand) - shimmerBand

	var b strings.Builder
	for _, p := range splitShimmer(line) {
		total := 0
		if p.marked {
			total = graphemeCount(p.text)
		}
		if total == 0 {
			b.WriteString(p.text)
			continue
		}
		writeShimmered(&b, p, total, center)
		b.WriteString(ansiFgReset)
	}
	return b.String()
}

func graphemeCount(s string) int {
	n := 0
	var state byte
	for len(s) > 0 {
		_, width, adv, newState := ansi.DecodeSequence(s, state, nil)
		if adv == 0 {
			break
		}
		if width > 0 {
			n++
		}
		state = newState
		s = s[adv:]
	}
	return n
}

func writeShimmered(b *strings.Builder, part shimmerPart, total int, center float64) {
	s := part.text
	idx := 0
	var state byte
	var last RGB
	hasLast := false
	for len(s) > 0 {
		seq, width, adv, newState := ansi.DecodeSequence(s, state, nil)
		if adv == 0 {
			break
		}
		if width > 0 {
			var p float64
			if total > 1 {
				p = float64(idx) / float64(total-1)
			}
			d := (p - center) / shimmerBand
			intensity := math.Exp(-d * d)
			c := lerpRGB(part.base, part.peak, intensity)
			if !hasLast || c != last {
				fmt.Fprintf(b, ansiFgTrueColorFmt, c.r, c.g, c.b)
				last = c
				hasLast = true
			}
			idx++
		} else if strings.HasPrefix(seq, "\x1b[") && strings.HasSuffix(seq, "m") {
			// An embedded SGR sequence may reset the foreground; re-emit after it.
			hasLast = false
		}
		b.WriteString(seq)
		state = newState
		s = s[adv:]
	}
}

func lerpRGB(a, c RGB, t float64) RGB {
	return NewRGB(
		a.r+int(math.Round(float64(c.r-a.r)*t)),
		a.g+int(math.Round(float64(c.g-a.g)*t)),
		a.b+int(math.Round(float64(c.b-a.b)*t)),
	)
}

func advanceShimmer(phase float64) float64 {
	phase += shimmerStep
	if phase >= 1+shimmerGap {
		phase = 0
	}
	return phase
}
