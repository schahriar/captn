package cog

import (
	"context"
	"fmt"
	"runtime/trace"
	"strings"
	"sync"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/languages"
	"github.com/schahriar/captn/pkg/lsp"
)

// TODO: Convert to LRU
var lspServerCache map[string]*lsp.Client = map[string]*lsp.Client{}
var lspServerCacheMu sync.Mutex

func loadLSPServerForLanguage(lang languages.LanguageSupport, workspace string) (*lsp.Client, error) {
	serkey := fmt.Sprintf("%v:%v", lang.GetLanguageID(), workspace)

	// Held across lsp.Start so concurrent callers for the same language share a
	// single server instead of racing on the map or spawning duplicates.
	lspServerCacheMu.Lock()
	defer lspServerCacheMu.Unlock()

	if server, ok := lspServerCache[serkey]; ok {
		return server, nil
	}

	client, err := lsp.Start(context.Background(), lsp.NewStartOptions(workspace, "captn-lsp-client", "0.1.0", lang.NewLSPServer))

	if err != nil {
		return nil, fmt.Errorf("failed to load language server for language %v with error %w", lang.GetLanguageID(), err)
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
		), pos)

		if err != nil && strings.Contains(err.Error(), "has no readable files") {
			continue
		}

		if err != nil {
			return []common.ResolvedDependency{}, fmt.Errorf("Failed to resolve imports for %+v: %w", imp, err)
		}

		for _, ref := range refs {
			ri, err := NewResolvedDependencyFromURIFromCOGFile(ctx, pf, pos, ref)
			if err != nil {
				return []common.ResolvedDependency{}, fmt.Errorf("Breaking path under workspace %v where import node = %+v\n and LSP ref = %+v\n %w", pf.Source.Workspace, imp, ref, err)
			}

			impLoc = append(impLoc, ri)
		}
	}

	reg.End()

	return impLoc, nil
}

func (pf *COGFile) FindNodesWithinRange(r *common.FileRange) []ast.ASTNode {
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
