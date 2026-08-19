package languages_swift

import (
	"bytes"
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

const sourcekitName = "sourcekit-lsp"

// NormalizeDefinitionRange widens sourcekit-lsp's reply to the extent of the
// identifier it points at.
//
// gopls and pyright answer textDocument/definition with the declared name's
// span. sourcekit-lsp answers with a zero-width position at the start of that
// name instead. A range spanning no bytes contains no nodes, so the definition
// would resolve to nothing at all. Widening here keeps the quirk in the one
// place that knows about sourcekit-lsp rather than in the shared graph code.
func (slsd *SwiftLanguageSupportDefinition) NormalizeDefinitionRange(src *common.Source, r *common.FileRange) *common.FileRange {
	if src == nil || r == nil || r.Start.BytePosition != r.End.BytePosition {
		return r
	}

	width := swiftIdentifierWidth(src.Buffer, r.Start.BytePosition)

	if width == 0 {
		return r
	}

	// An identifier never spans a newline, so the line is unchanged and the
	// column advances by the same byte count (Source treats columns as bytes
	// into the line, see BytePositionForLineColumn)
	end := r.End
	end.BytePosition += width
	end.Column += width

	return common.NewFileRange(r.Source, r.Start, end)
}

// swiftIdentifierWidth measures in bytes the identifier starting at `at`,
// answering 0 when there is none. Backtick escaping (`func `default`()`) is
// covered because sourcekit-lsp points at the opening backtick.
func swiftIdentifierWidth(buf []byte, at int) int {
	if at < 0 || at >= len(buf) {
		return 0
	}

	if buf[at] == '`' {
		if closing := bytes.IndexByte(buf[at+1:], '`'); closing >= 0 {
			return closing + 2
		}

		return 0
	}

	width := 0

	for at+width < len(buf) {
		r, size := utf8.DecodeRune(buf[at+width:])

		if size == 0 || (r == utf8.RuneError && size <= 1) {
			break
		}

		// Swift identifiers are Unicode; digits are legal after the first rune
		// and sourcekit-lsp always points at the first
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			break
		}

		width += size
	}

	return width
}

// sourcekitInstallCommand answers with the command that installs a Swift
// toolchain, which is what actually ships sourcekit-lsp. Unlike gopls and
// pyright there is no single cross-platform installer, so the platform picks.
func sourcekitInstallCommand() string {
	if runtime.GOOS == "darwin" {
		return "xcode-select --install"
	}

	return "swiftly install latest"
}

func sourcekitPath(ctx context.Context) (string, error) {
	binary := sourcekitName

	if runtime.GOOS == "windows" {
		binary += ".exe"
	}

	if path, err := exec.LookPath(binary); err == nil {
		return path, nil
	}

	// On macOS the toolchain is inside the Xcode bundle and never on PATH, so
	// the toolchain is asked where it put the binary rather than guessing at
	// the bundle layout
	if runtime.GOOS == "darwin" {
		if path := xcrunPath(ctx); path != "" {
			return path, nil
		}
	}

	// Every other distribution (swiftly, the swift.org tarballs, the Windows
	// installer) ships sourcekit-lsp beside swift itself
	if path := swiftSiblingPath(binary); path != "" {
		return path, nil
	}

	return "", fmt.Errorf("%v was not found in PATH or alongside the active Swift toolchain", sourcekitName)
}

func xcrunPath(ctx context.Context) string {
	out, err := exec.CommandContext(ctx, "xcrun", "--find", sourcekitName).Output()

	if err != nil {
		return ""
	}

	path := strings.TrimSpace(string(out))

	if path == "" {
		return ""
	}

	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return ""
	}

	return path
}

func swiftSiblingPath(binary string) string {
	swift := "swift"

	if runtime.GOOS == "windows" {
		swift += ".exe"
	}

	swiftPath, err := exec.LookPath(swift)

	if err != nil {
		return ""
	}

	// LookPath can answer with a symlink into a version manager's shim
	// directory; the real toolchain is where it points
	if resolved, err := filepath.EvalSymlinks(swiftPath); err == nil {
		swiftPath = resolved
	}

	candidate := filepath.Join(filepath.Dir(swiftPath), binary)

	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		return candidate
	}

	return ""
}
