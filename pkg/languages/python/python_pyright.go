package languages_python

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/schahriar/captn/pkg/common"
)

const (
	pyrightServerName     = "pyright-langserver"
	pyrightInstallCommand = "npm install -g pyright"
)

func pyrightPath(ctx context.Context) (string, error) {
	// LookPath also resolves npm's .cmd shims on Windows via PATHEXT
	if path, err := exec.LookPath(pyrightServerName); err == nil {
		return path, nil
	}

	// `npm install -g` drops binaries somewhere that is often not on PATH,
	// so the global install directory is checked too rather than asking for
	// an install that already happened.
	if dir := npmGlobalBinDir(ctx); dir != "" {
		candidates := []string{pyrightServerName}

		if runtime.GOOS == "windows" {
			candidates = []string{pyrightServerName + ".cmd", pyrightServerName + ".exe"}
		}

		for _, name := range candidates {
			candidate := filepath.Join(dir, name)

			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
		}
	}

	return "", fmt.Errorf("%v was not found in PATH or the npm global bin directory", pyrightServerName)
}

// npmGlobalBinDir asks npm where global binaries land: bin/ under the prefix
// on Unix, the prefix itself on Windows. npm is asked rather than guessing
// because the prefix moves with nvm and per-user configuration.
func npmGlobalBinDir(ctx context.Context) string {
	out, err := exec.CommandContext(ctx, "npm", "prefix", "-g").Output()

	if err != nil {
		return ""
	}

	prefix := strings.TrimSpace(string(out))

	if prefix == "" {
		return ""
	}

	if runtime.GOOS == "windows" {
		return prefix
	}

	return filepath.Join(prefix, "bin")
}

// NormalizeDefinitionRange narrows pyright's reply to the first name in it.
// pyright answers most definitions with the declared name's span but a
// parameter, a typing special form or a type parameter list with the whole
// node, and a range holding several nodes resolves to none of them. Empty
// ranges pass through: an import resolves to 0:0 of its module.
func (plsd *PythonLanguageSupportDefinition) NormalizeDefinitionRange(src *common.Source, r *common.FileRange) *common.FileRange {
	if src == nil || r == nil || r.Start.BytePosition >= r.End.BytePosition {
		return r
	}

	at, width := firstIdentifier(src.Buffer, r.Start.BytePosition, r.End.BytePosition)

	if width == 0 || (at == r.Start.BytePosition && at+width == r.End.BytePosition) {
		return r
	}

	start := advance(src.Buffer, r.Start, at)
	end := advance(src.Buffer, start, at+width)

	return common.NewFileRange(r.Source, start, end)
}

// Returns the identifier's start and byte width, width 0 when there is none
func firstIdentifier(buf []byte, from int, to int) (int, int) {
	to = min(to, len(buf))
	at := max(from, 0)

	for at < to {
		r, size := utf8.DecodeRune(buf[at:to])

		if isIdentifierRune(r) {
			break
		}

		at += size
	}

	width := 0

	for at+width < to {
		r, size := utf8.DecodeRune(buf[at+width : to])

		if !isIdentifierRune(r) {
			break
		}

		width += size
	}

	return at, width
}

func isIdentifierRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func (plsd *PythonLanguageSupportDefinition) GetLSPInitializationOptions(_ context.Context, _ string) any {
	return nil
}

// Source treats columns as bytes into the line, see BytePositionForLineColumn
func advance(buf []byte, pos common.FilePosition, to int) common.FilePosition {
	for pos.BytePosition < to {
		if buf[pos.BytePosition] == '\n' {
			pos.Line++
			pos.Column = 0
		} else {
			pos.Column++
		}

		pos.BytePosition++
	}

	return pos
}
