package cog

import (
	"context"
	"fmt"
	"sync"

	"github.com/dominikbraun/graph"
	"github.com/schahriar/captn/pkg/common"
)

type COG struct {
	Graph            graph.Graph[string, COGNode]
	mux              sync.Mutex
	loadedFiles      map[string]*COGFile
	Workspace        string
	ObservationCache map[string]common.ObservationSchema
}

// COGNode - COG has polymorphic nodes as long as they conform to this interface
type COGNode interface {
	GetHash() string
	GetSource() string
	ListImports(ctx context.Context) (common.ResolvedImports, error)
}

func NodeHasher(cogn COGNode) string {
	return cogn.GetHash()
}

func NewCOG(workspace string) *COG {
	return &COG{
		Graph:       graph.New(NodeHasher),
		mux:         sync.Mutex{},
		loadedFiles: map[string]*COGFile{},
		Workspace:   workspace,
	}
}

func (cog *COG) LoadFile(ctx context.Context, file string) (*COGFile, error) {
	cog.mux.Lock()

	if n, ok := cog.loadedFiles[file]; ok {
		cog.mux.Unlock()
		return n, nil
	}

	cog.mux.Unlock()

	f, err := ParseFile(ctx, cog.Workspace, file)

	if err != nil {
		return nil, fmt.Errorf("failed to parse file (%v) %w", file, err)
	}

	cog.mux.Lock()
	cog.loadedFiles[file] = f

	cog.Graph.AddVertex(f, graph.VertexAttribute("import_type", string(f.Language.ClassifyImportType(f.Source))))
	cog.mux.Unlock()

	return f, nil
}

func (cog *COG) queryWithDepth(ctx context.Context, g graph.Graph[string, COGNode], n COGNode, depth int, mux *sync.Mutex, visited map[string]bool) error {
	if depth <= 0 || visited[n.GetHash()] {
		return nil
	}

	visited[n.GetHash()] = true

	switch v := n.(type) {
	// TODO: Support querying other sub nodes
	case *COGFile:
		mux.Lock()
		g.AddVertex(n, graph.VertexAttribute("import_type", string(v.Language.ClassifyImportType(v.Source))))
		mux.Unlock()

		imps, err := v.ListImports(ctx)

		if err != nil {
			return err
		}

		var wg sync.WaitGroup
		errs := []error{}

		for _, imp := range imps {
			wg.Go(func() {
				f, err := cog.LoadFile(ctx, imp.External.Source.Path)

				if err != nil {
					mux.Lock()
					errs = append(errs, err)
					mux.Unlock()
					return
				}

				t := f.Language.ClassifyImportType(f.Source)

				if t == common.ImportStandardLibrary || t == common.ImportDependency {
					return
				}

				mux.Lock()
				g.AddVertex(f, graph.VertexAttribute("import_type", string(t)))
				if err := g.AddEdge(n.GetHash(), f.GetHash()); err != nil {
					errs = append(errs, err)
					mux.Unlock()
					return
				}
				mux.Unlock()

				if err := cog.queryWithDepth(ctx, g, f, depth-1, mux, visited); err != nil {
					mux.Lock()
					errs = append(errs, err)
					mux.Unlock()
				}
			})
		}

		wg.Wait()

		if len(errs) > 0 {
			return fmt.Errorf("failed with %v errors %w", len(errs), errs[0])
		}

		return nil
	default:
		return fmt.Errorf("can't explain unknown node type %T", v)
	}
}

func (cog *COG) QueryWithDepth(ctx context.Context, n COGNode, depth int) (graph.Graph[string, COGNode], error) {
	g := graph.New(NodeHasher)
	return g, cog.queryWithDepth(ctx, g, n, depth, &sync.Mutex{}, map[string]bool{})
}

func (cog *COG) ExplainWithDepth(ctx context.Context, n COGNode, prov ObservationProvider, depth int) (string, error) {
	g := graph.New(NodeHasher)

	if err := cog.queryWithDepth(ctx, g, n, depth, &sync.Mutex{}, map[string]bool{}); err != nil {
		return "", err
	}

	if err := prov.ResolveObservationsToGraph(ctx, g, n); err != nil {
		return "", err
	}

	/*cog.mux.Lock()
	cog.ObservationCache[h] = o
	cog.mux.Unlock()*/

	return "", nil
}
