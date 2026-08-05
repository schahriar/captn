package cog

import (
	"bytes"
	"context"
	"fmt"

	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/lsp"
)

func safeReadAttr(m map[string]string, k string) string {
	if v, ok := m[k]; ok {
		return v
	}

	return ""
}

func NewResolvedDependencyFromURIFromCOGFile(
	ctx context.Context,
	file *COGFile,
	internal *common.FileRange,
	ref lsp.Location,
) (common.ResolvedDependency, error) {
	return common.NewResolvedDependencyFromURI(
		ctx,
		file.Source.Workspace,
		internal,
		ref.URI,
		ref.Range.Start.Line,
		ref.Range.Start.Character,
		ref.Range.End.Line,
		ref.Range.End.Character,
		file.Language.ClassifyImportType,
	)
}

func ReplaceNewlines(input, replacement []byte) []byte {
	output := make([]byte, 0, len(input))

	for i := 0; i < len(input); i++ {
		switch input[i] {
		case '\r':
			// Treat CRLF as one newline.
			if i+1 < len(input) && input[i+1] == '\n' {
				i++
			}
			output = append(output, replacement...)

		case '\n':
			output = append(output, replacement...)

		default:
			output = append(output, input[i])
		}
	}

	// Postcondition: the output cannot contain CR or LF.
	if bytes.IndexByte(output, '\r') >= 0 ||
		bytes.IndexByte(output, '\n') >= 0 {
		panic(fmt.Sprintf("ReplaceNewlines postcondition failed: output contains CR or LF: %q", output))
	}

	return output
}
