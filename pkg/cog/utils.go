package cog

import (
	"context"

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
