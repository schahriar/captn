package common

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime/trace"
)

type Source struct {
	Workspace string
	Path      string
	Buffer    []byte `json:"-"`
}

func NewSource(workspace, path string, buf []byte) *Source {
	return &Source{
		Workspace: workspace,
		Path:      path,
		Buffer:    buf,
	}
}

func (src *Source) BytePositionForLineColumn(line int, col int) (int, error) {
	if src == nil {
		return 0, fmt.Errorf("source is nil")
	}

	if line < 0 {
		return 0, fmt.Errorf("line cannot be negative")
	}

	if col < 0 {
		return 0, fmt.Errorf("column cannot be negative")
	}

	currentLine := 0
	lineStart := 0

	for i, b := range src.Buffer {
		if currentLine == line {
			lineEnd := len(src.Buffer)

			for j := i; j < len(src.Buffer); j++ {
				if src.Buffer[j] == '\n' {
					lineEnd = j

					if lineEnd > lineStart && src.Buffer[lineEnd-1] == '\r' {
						lineEnd--
					}

					break
				}
			}

			if lineStart+col > lineEnd {
				return 0, fmt.Errorf("column %d is out of range for line %d", col, line)
			}

			return lineStart + col, nil
		}

		if b == '\n' {
			currentLine++
			lineStart = i + 1
		}
	}

	if currentLine == line {
		if lineStart+col > len(src.Buffer) {
			return 0, fmt.Errorf("column %d is out of range for line %d", col, line)
		}

		return lineStart + col, nil
	}

	return 0, fmt.Errorf("line %d is out of range", line)
}

// RelativePath returns Path relative to the workspace root with forward
// slashes, so identity derived from it is stable across checkout locations
// and operating systems. Paths outside the workspace or with no workspace
// fall back to the slash-normalized Path.
func (src *Source) RelativePath() string {
	if src.Workspace != "" {
		if rel, err := filepath.Rel(src.Workspace, src.Path); err == nil {
			return filepath.ToSlash(rel)
		}
	}

	return filepath.ToSlash(src.Path)
}

func (src *Source) GetLanguage() string {
	// TODO: Implement a better version
	switch filepath.Ext(src.Path) {
	case ".go":
		return "golang"
	case ".py", ".pyi":
		return "python"
	case ".swift", ".swiftinterface":
		return "swift"
	case ".ts", ".mts", ".cts", ".tsx":
		return "typescript"
	case ".js", ".mjs", ".cjs", ".jsx":
		return "javascript"
	case ".css":
		return "css"
	case ".scss":
		return "scss"
	case ".less":
		return "less"
	case ".html":
		return "html"
	default:
		return "unknown"
	}
}

func NewSourceFromFile(ctx context.Context, workspace string, path string) (*Source, error) {
	_, task := trace.NewTask(ctx, "loadFile")

	defer task.End()

	buf, err := os.ReadFile(path)

	if err != nil {
		return nil, err
	}

	return NewSource(workspace, path, buf), nil
}
