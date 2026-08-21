package languages_typescript

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
	tsserverName = "typescript-language-server"
	// Pinned to typescript 5: the 7.x native rewrite no longer ships the
	// lib/tsserver.js this server runs
	tsserverInstallCommand = "npm install -g typescript-language-server typescript@5"
)

func tsserverPath(ctx context.Context) (string, error) {
	binary, err := tsserverBinaryPath(ctx)

	if err != nil {
		return "", err
	}

	// The server is unusable without a typescript install to run: it exits
	// at initialize, so a missing one is reported here where the install
	// command reaches the agent
	if tsserverJSPath(ctx, "") == "" {
		return "", fmt.Errorf("no typescript install with lib/tsserver.js was found for %v to run", tsserverName)
	}

	return binary, nil
}

func tsserverBinaryPath(ctx context.Context) (string, error) {
	// LookPath also resolves npm's .cmd shims on Windows via PATHEXT
	if path, err := exec.LookPath(tsserverName); err == nil {
		return path, nil
	}

	// `npm install -g` drops binaries somewhere that is often not on PATH,
	// so the global install directory is checked too rather than asking for
	// an install that already happened.
	if dir := npmGlobalBinDir(ctx); dir != "" {
		candidates := []string{tsserverName}

		if runtime.GOOS == "windows" {
			candidates = []string{tsserverName + ".cmd", tsserverName + ".exe"}
		}

		for _, name := range candidates {
			candidate := filepath.Join(dir, name)

			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate, nil
			}
		}
	}

	return "", fmt.Errorf("%v was not found in PATH or the npm global bin directory", tsserverName)
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

// tsserverJSPath resolves the tsserver.js the language server should run:
// the workspace's own typescript when installed, walking up like node module
// resolution does, otherwise the globally installed one.
func tsserverJSPath(ctx context.Context, workspace string) string {
	if abs, err := filepath.Abs(workspace); err == nil {
		for dir := abs; ; {
			candidate := filepath.Join(dir, "node_modules", "typescript", "lib", "tsserver.js")

			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}

			parent := filepath.Dir(dir)

			if parent == dir {
				break
			}

			dir = parent
		}
	}

	out, err := exec.CommandContext(ctx, "npm", "root", "-g").Output()

	if err != nil {
		return ""
	}

	root := strings.TrimSpace(string(out))

	if root == "" {
		return ""
	}

	candidate := filepath.Join(root, "typescript", "lib", "tsserver.js")

	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		return candidate
	}

	return ""
}

// typescript-language-server exits at initialize unless the workspace holds
// its own typescript install or tsserver.path points at one
func (tlsd *TypescriptLanguageSupportDefinition) GetLSPInitializationOptions(ctx context.Context, workspace string) any {
	path := tsserverJSPath(ctx, workspace)

	if path == "" {
		return nil
	}

	return map[string]any{
		"tsserver": map[string]any{
			"path": path,
		},
	}
}

// NormalizeDefinitionRange narrows tsserver's reply to the first name in it.
// Most definitions answer with the declared name's span, but a callable
// interface answers with its whole call or construct signature, and a range
// holding several nodes resolves to none of them. Keywords are skipped over:
// no node can sit on `new` or a predefined type, and a signature made only of
// those (`(): void`) passes through whole, where the member's own node spans
// it exactly. Empty ranges pass through: an import resolves to 0:0 of its
// module.
func (tlsd *TypescriptLanguageSupportDefinition) NormalizeDefinitionRange(src *common.Source, r *common.FileRange) *common.FileRange {
	if src == nil || r == nil || r.Start.BytePosition >= r.End.BytePosition {
		return r
	}

	at := r.Start.BytePosition
	width := 0

	for at < r.End.BytePosition {
		at, width = firstIdentifier(src.Buffer, at, r.End.BytePosition)

		if width == 0 {
			return r
		}

		if !isSkippableKeyword(string(src.Buffer[at : at+width])) {
			break
		}

		at += width
		width = 0
	}

	if width == 0 || (at == r.Start.BytePosition && at+width == r.End.BytePosition) {
		return r
	}

	start := advance(src.Buffer, r.Start, at)
	end := advance(src.Buffer, start, at+width)

	return common.NewFileRange(r.Source, start, end)
}

func isSkippableKeyword(word string) bool {
	switch word {
	case "new", "readonly", "this", "keyof", "typeof", "infer",
		"any", "bigint", "boolean", "never", "null", "number", "object",
		"string", "symbol", "undefined", "unknown", "void":
		return true
	}
	return false
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
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '$'
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
