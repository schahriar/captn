package cog

import (
	"context"
	"runtime/trace"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/lsp"
)

func (pf ParsedFile) ListImports(ctx context.Context) ([]lsp.Location, error) {
	reg := trace.StartRegion(ctx, "ListImports:LSPServerLoad")

	client, err := lsp.Start(ctx, lsp.StartOptions{
		WorkspaceRoot: pf.Source.Workspace,
		ClientName:    "captn-lsp-client",
		ClientVersion: "0.1.0",
		Spawn:         pf.Language.NewLSPServer,
	})

	if err != nil {
		return []lsp.Location{}, err
	}

	reg.End()

	defer client.Close(ctx)

	reg = trace.StartRegion(ctx, "LSPImportsQuery")

	impVis := &ast.ImportVisitor{}

	pf.Module.Accept(impVis)

	impLoc := []lsp.Location{}

	for _, imp := range impVis.Imports {
		pos := imp.GetPosition()

		refs, err := client.ImportDefinition(ctx, lsp.TextDocumentItem{
			URI:        lsp.FileURI(pf.Source.Path),
			LanguageID: pf.Language.GetLanguageID(),
			Version:    1,
			Text:       string(pf.Source.Buffer),
		}, pos)

		if err != nil {
			panic(err)
		}

		for _, ref := range refs {
			impLoc = append(impLoc, ref)
		}
	}

	reg.End()

	return impLoc, nil
}
