package grep

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

type Match struct {
	Path string
	Line int
	Text string
}

func NewMatch(path string, line int, text string) Match {
	return Match{
		Path: path,
		Line: line,
		Text: text,
	}
}

func ParseGrepOutput(r io.Reader) ([]Match, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read grep output: %w", err)
	}

	if bytes.IndexByte(data, 0) >= 0 {
		return parseNULDelimited(data)
	}

	return parseColonDelimited(data)
}

// parseNULDelimited parses the GNU "-Z" dialect: "path\x00line:text\n" records.
func parseNULDelimited(data []byte) ([]Match, error) {
	var matches []Match

	for len(data) > 0 {
		nul := bytes.IndexByte(data, 0)
		if nul < 0 {
			break
		}

		path := string(data[:nul])
		data = data[nul+1:]

		var rec []byte
		if nl := bytes.IndexByte(data, '\n'); nl >= 0 {
			rec, data = data[:nl], data[nl+1:]
		} else {
			rec, data = data, nil
		}

		colon := bytes.IndexByte(rec, ':')
		if colon < 0 {
			continue
		}

		lineNo, err := strconv.Atoi(string(rec[:colon]))
		if err != nil {
			continue
		}

		matches = append(matches, NewMatch(path, lineNo, string(rec[colon+1:])))
	}

	return matches, nil
}

// parseColonDelimited parses the BSD "path:line:text" dialect line by line.
func parseColonDelimited(data []byte) ([]Match, error) {
	sc := bufio.NewScanner(bytes.NewReader(data))
	// Matched lines can be long; raise the token limit well above the 64KB default.
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	var matches []Match

	for sc.Scan() {
		if m, ok := parseColonLine(sc.Text()); ok {
			matches = append(matches, m)
		}
	}

	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan grep output: %w", err)
	}

	return matches, nil
}

func parseColonLine(line string) (Match, bool) {
	zero := NewMatch("", 0, "")
	start := 0
	if len(line) >= 3 && isDriveLetter(line[0]) && line[1] == ':' && (line[2] == '\\' || line[2] == '/') {
		start = 2
	}

	rel := strings.IndexByte(line[start:], ':')
	if rel < 0 {
		return zero, false
	}
	pathEnd := start + rel

	rest := line[pathEnd+1:]
	colon := strings.IndexByte(rest, ':')
	if colon < 0 {
		return zero, false
	}

	lineNo, err := strconv.Atoi(rest[:colon])
	if err != nil {
		return zero, false
	}

	return NewMatch(line[:pathEnd], lineNo, rest[colon+1:]), true
}

func isDriveLetter(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

func SearchSource(ctx context.Context, workdir string, include string, pattern string) ([]Match, error) {
	cmd := exec.CommandContext(
		ctx,
		"grep",
		"-RInFZH",
		fmt.Sprintf("--include=%s", include),
		pattern,
		workdir,
	)

	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return nil, nil
		}

		return nil, err
	}

	return ParseGrepOutput(bytes.NewReader(out))
}

func Search(ctx context.Context, workdir string, pattern string) ([]Match, error) {
	cmd := exec.CommandContext(
		ctx,
		"grep",
		"-RInFZH",
		pattern,
		workdir,
	)

	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
			return nil, nil
		}

		return nil, err
	}

	return ParseGrepOutput(bytes.NewReader(out))
}
