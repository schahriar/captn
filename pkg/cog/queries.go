package cog

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime/trace"
	"strings"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/languages"
	"github.com/schahriar/captn/pkg/lsp"
)

// TODO: Convert to LRU
var lspServerCache map[string]*lsp.Client = map[string]*lsp.Client{}

func loadLSPServerForLanguage(lang languages.LanguageSupport, workspace string) (*lsp.Client, error) {
	serkey := fmt.Sprintf("%v:%v", lang.GetLanguageID(), workspace)
	if server, ok := lspServerCache[serkey]; ok {
		return server, nil
	}

	client, err := lsp.Start(context.Background(), lsp.NewStartOptions(workspace, "captn-lsp-client", "0.1.0", lang.NewLSPServer))

	if err != nil {
		return nil, err
	}

	lspServerCache[serkey] = client

	return client, nil
}

func (pf *COGFile) ListDependencies(ctx context.Context) (common.ResolvedDependencies, error) {
	reg := trace.StartRegion(ctx, "ListDependencies:LSPServerLoad")

	client, err := loadLSPServerForLanguage(pf.Language, pf.Source.Workspace)

	if err != nil {
		return []common.ResolvedDependency{}, err
	}

	reg.End()

	reg = trace.StartRegion(ctx, "LSPImportsQuery")

	impVis := ast.NewImportVisitor()

	pf.Module.Accept(impVis)

	impLoc := []common.ResolvedDependency{}

	for _, imp := range impVis.Imports {
		pos := imp.GetPosition()

		if pos == nil {
			return []common.ResolvedDependency{}, fmt.Errorf("Position was nil for import node %+v\n", imp)
		}

		refs, err := client.ImportDefinition(ctx, lsp.NewTextDocumentItem(
			lsp.FileURI(pf.Source.Path),
			pf.Language.GetLanguageID(),
			1,
			string(pf.Source.Buffer),
		), *pos)

		if err != nil && strings.Contains(err.Error(), "has no readable files") {
			continue
		}

		if err != nil {
			return []common.ResolvedDependency{}, fmt.Errorf("Failed to resolve imports for %+v: %w", imp, err)
		}

		for _, ref := range refs {
			refp, err := lsp.AbsolutePathFromURI(ref.URI)

			if err != nil {
				return []common.ResolvedDependency{}, fmt.Errorf("Breaking path under workspace %v where import node = %+v\n and LSP ref = %+v\n %w", pf.Source.Workspace, imp, ref, err)
			}

			rel, err := filepath.Rel(pf.Source.Workspace, refp)

			if err != nil {
				return []common.ResolvedDependency{}, fmt.Errorf("Breaking path under workspace %v where import node = %+v\n and LSP ref = %+v\n %w", pf.Source.Workspace, imp, ref, err)
			}

			esrc, err := common.NewSourceFromFile(ctx, pf.Source.Workspace, rel)

			if err != nil {
				return []common.ResolvedDependency{}, err
			}

			erange, err := common.NewFileRangeAutoBytePosition(esrc, ref.Range.Start.Line, ref.Range.Start.Character, ref.Range.End.Line, ref.Range.End.Character)

			if err != nil {
				return []common.ResolvedDependency{}, err
			}

			ri := common.NewResolvedDependency(pf.Language.ClassifyImportType(erange.Source), pos, erange)
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
