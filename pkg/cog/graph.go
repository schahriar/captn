package cog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/dominikbraun/graph"
	"github.com/schahriar/captn/pkg/cgraph"
	"github.com/schahriar/captn/pkg/common"
)

type COG struct {
	loadedFiles map[string]*COGFile `json:""`

	Mux *sync.Mutex `json:""`

	Graph            graph.Graph[string, COGNode] `json:""`
	Workspace        string
	ObservationCache map[string]common.ObservationSchema
}

// COGNode - COG has polymorphic nodes as long as they conform to this interface
type COGNode interface {
	GetHash() string
	GetFilePath() string
	GetLanguage() string
	GetSource() string
	ListDependencies(ctx context.Context) (common.ResolvedDependencies, error)
}

func NodeHasher(cogn COGNode) string {
	return cogn.GetHash()
}

func NewCOG(workspace string) *COG {
	return &COG{
		Graph:            graph.New(NodeHasher),
		Mux:              &sync.Mutex{},
		loadedFiles:      map[string]*COGFile{},
		ObservationCache: map[string]common.ObservationSchema{},
		Workspace:        workspace,
	}
}

func (cog *COG) FilePath() string {
	return filepath.Join(cog.Workspace, "captn.cog")
}

func OpenCOG(workspace string) (*COG, error) {
	cog := NewCOG(workspace)

	f, err := os.OpenFile(cog.FilePath(), os.O_RDWR, 0644)

	if errors.Is(err, os.ErrNotExist) {
		return cog, nil
	} else if err != nil {
		return nil, fmt.Errorf("OpenCOG File Error %w", err)
	}

	defer f.Close()

	b, err := io.ReadAll(f)

	if err != nil {
		return nil, fmt.Errorf("OpenCOG File Read Error %w", err)
	}

	return cog, json.Unmarshal(b, &cog)
}

func (cog *COG) Persist() error {
	b, err := json.Marshal(cog)

	if err != nil {
		return err
	}

	return os.WriteFile(cog.FilePath(), b, 0644)
}

func (cog *COG) LoadFile(ctx context.Context, file string) (*COGFile, error) {
	cog.Mux.Lock()

	if n, ok := cog.loadedFiles[file]; ok {
		cog.Mux.Unlock()
		return n, nil
	}

	cog.Mux.Unlock()

	f, err := ParseFile(ctx, cog.Workspace, file)

	if err != nil {
		return nil, fmt.Errorf("failed to parse file (%v) %w", file, err)
	}

	cog.Mux.Lock()
	cog.loadedFiles[file] = f

	cog.Graph.AddVertex(f, graph.VertexAttribute("import_type", string(f.Language.ClassifyImportType(f.Source))))
	cog.Mux.Unlock()

	return f, nil
}

func (cog *COG) QuerySnippet(ctx context.Context, file string, snippet string) (*ObservationGraph, COGNode, error) {
	f, err := cog.LoadFile(ctx, file)

	if err != nil {
		return nil, nil, err
	}

	_, err = f.FindSnippetRange([]byte(snippet))

	if err != nil {
		return nil, nil, err
	}

	// chlds := f.QueryNodesWithinRange(r)
	/*root := &ast.ASTModule{
		Name: file,
	}*/

	g := cgraph.New(NodeHasher)
	og := &ObservationGraph{
		Graph: &g,
	}

	return og, nil, nil
}

func (cog *COG) queryWithDepth(ctx context.Context, g *ObservationGraph, n COGNode, depth int, mux *sync.Mutex, visited map[string]bool) error {
	if depth <= 0 || visited[n.GetHash()] {
		return nil
	}

	visited[n.GetHash()] = true

	switch v := n.(type) {
	// TODO: Support querying other sub nodes
	case *COGFile:
		mux.Lock()
		g.Graph.AddVertex(n, graph.VertexAttribute("import_type", string(v.Language.ClassifyImportType(v.Source))))
		mux.Unlock()

		imps, err := v.ListDependencies(ctx)

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

				if t == common.StandardLibraryDependency || t == common.PackageDependency {
					return
				}

				mux.Lock()
				g.Graph.AddVertex(f, graph.VertexAttribute("import_type", string(t)))
				if err := g.Graph.AddEdge(n.GetHash(), f.GetHash()); err != nil {
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

func (cog *COG) QueryWithDepth(ctx context.Context, n COGNode, depth int) (*ObservationGraph, error) {
	g := cgraph.New(NodeHasher)
	og := &ObservationGraph{
		Graph: &g,
	}
	return og, cog.queryWithDepth(ctx, og, n, depth, &sync.Mutex{}, map[string]bool{})
}
