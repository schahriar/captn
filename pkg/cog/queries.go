package cog

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime/trace"
	"strings"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/lsp"
)

type ResolvedImport struct {
	Internal *common.FileRange
	External *common.FileRange
}

type ResolvedImports []ResolvedImport

func (ri ResolvedImports) GroupByPackage() map[*common.FileRange]ResolvedImports {
	groups := make(map[*common.FileRange]ResolvedImports)
	for _, imp := range ri {
		groups[imp.Internal] = append(groups[imp.Internal], imp)
	}
	return groups
}

func (pf *COGFile) ListImports(ctx context.Context) (ResolvedImports, error) {
	reg := trace.StartRegion(ctx, "ListImports:LSPServerLoad")

	client, err := lsp.Start(ctx, lsp.StartOptions{
		WorkspaceRoot: pf.Source.Workspace,
		ClientName:    "captn-lsp-client",
		ClientVersion: "0.1.0",
		Spawn:         pf.Language.NewLSPServer,
	})

	if err != nil {
		return []ResolvedImport{}, err
	}

	reg.End()

	defer client.Close(ctx)

	reg = trace.StartRegion(ctx, "LSPImportsQuery")

	impVis := &ast.ImportVisitor{}

	pf.Module.Accept(impVis)

	impLoc := []ResolvedImport{}

	for _, imp := range impVis.Imports {
		pos := imp.GetPosition()

		if pos == nil {
			return []ResolvedImport{}, fmt.Errorf("Position was nil for import node %+v\n", imp)
		}

		refs, err := client.ImportDefinition(ctx, lsp.TextDocumentItem{
			URI:        lsp.FileURI(pf.Source.Path),
			LanguageID: pf.Language.GetLanguageID(),
			Version:    1,
			Text:       string(pf.Source.Buffer),
		}, *pos)

		if err != nil && strings.Contains(err.Error(), "has no readable files") {
			continue
		}

		if err != nil {
			return []ResolvedImport{}, fmt.Errorf("Failed to resolve imports for %+v: %w", imp, err)
		}

		for _, ref := range refs {
			refp, err := lsp.AbsolutePathFromURI(ref.URI)

			if err != nil {
				return []ResolvedImport{}, fmt.Errorf("Breaking path under workspace %v where import node = %+v\n and LSP ref = %+v\n %w", pf.Source.Workspace, imp, ref, err)
			}

			rel, err := filepath.Rel(pf.Source.Workspace, refp)

			if err != nil {
				return []ResolvedImport{}, fmt.Errorf("Breaking path under workspace %v where import node = %+v\n and LSP ref = %+v\n %w", pf.Source.Workspace, imp, ref, err)
			}

			esrc, err := common.NewSourceFromFile(ctx, pf.Source.Workspace, rel)

			if err != nil {
				return []ResolvedImport{}, err
			}

			erange, err := common.NewFileRangeAutoBytePosition(esrc, ref.Range.Start.Line, ref.Range.Start.Character, ref.Range.End.Line, ref.Range.End.Character)

			if err != nil {
				return []ResolvedImport{}, err
			}

			ri := ResolvedImport{
				Internal: pos,
				External: erange,
			}
			impLoc = append(impLoc, ri)
		}
	}

	reg.End()

	return impLoc, nil
}

func (pf *COGFile) QueryNodesWithinRange(r *common.FileRange) []ast.ASTNode {
	hashes, ok := pf.intervals.AllIntersections(r.Start, r.End)

	if !ok {
		return []ast.ASTNode{}
	}

	nodes := make([]ast.ASTNode, 0, len(hashes))

	for _, hash := range hashes {
		node, ok := pf.lookupTable[hash]
		if !ok {
			continue
		}

		if r != nil {
			if node.GetPosition().ContainedBy(*r) {
				nodes = append(nodes, node)
			}
		}
	}

	return nodes
}
