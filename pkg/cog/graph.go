package cog

import (
	"context"
	"fmt"
	"sync"

	"github.com/dominikbraun/graph"
)

type COG struct {
	Graph       graph.Graph[string, COGNode]
	mux         sync.Mutex
	loadedFiles map[string]*COGFile
	Workspace   string
}

// COGNode - COG has polymorphic nodes as long as they conform to this interface
type COGNode interface {
	GetHash() string
	ListImports(ctx context.Context) (ResolvedImports, error)
}

func NewCOG(workspace string) *COG {
	return &COG{
		Graph: graph.New(func(cogn COGNode) string {
			return cogn.GetHash()
		}),
		mux:         sync.Mutex{},
		loadedFiles: map[string]*COGFile{},
		Workspace:   workspace,
	}
}

type importLoadResult struct {
	nodes []COGNode
	err   error
}

func (cog *COG) LoadImports(ctx context.Context, node COGNode, depth int) ([]COGNode, error) {
	visited := map[string]struct{}{}
	var visitedMux sync.Mutex

	return cog.loadImports(ctx, node, depth, visited, &visitedMux)
}

func (cog *COG) loadImports(
	ctx context.Context,
	node COGNode,
	depth int,
	visited map[string]struct{},
	visitedMux *sync.Mutex,
) ([]COGNode, error) {
	if depth <= 0 {
		return nil, nil
	}

	imports, err := node.ListImports(ctx)
	if err != nil {
		return nil, err
	}

	results := make(chan importLoadResult, len(imports))

	var wg sync.WaitGroup

	for _, imp := range imports {
		innerimp := imp // important for goroutine capture safety

		path := innerimp.External.Source.Path
		if path == "" {
			continue
		}

		visitedMux.Lock()
		if _, ok := visited[path]; ok {
			visitedMux.Unlock()
			continue
		}
		visited[path] = struct{}{}
		visitedMux.Unlock()

		wg.Add(1)

		go func() {
			defer wg.Done()

			fileNode, err := cog.LoadFile(ctx, path)
			if err != nil {
				results <- importLoadResult{err: err}
				return
			}

			nodes := []COGNode{fileNode}

			cog.mux.Lock()

			cog.Graph.AddVertex(fileNode)
			cog.Graph.AddEdge(node.GetHash(), fileNode.GetHash())

			cog.mux.Unlock()

			children, err := cog.loadImports(ctx, fileNode, depth-1, visited, visitedMux)
			if err != nil {
				results <- importLoadResult{err: err}
				return
			}

			nodes = append(nodes, children...)

			results <- importLoadResult{nodes: nodes}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var loaded []COGNode

	for res := range results {
		if res.err != nil {
			return nil, res.err
		}

		loaded = append(loaded, res.nodes...)
	}

	return loaded, nil
}

func (cog *COG) LoadFile(ctx context.Context, file string) (*COGFile, error) {
	cog.mux.Lock()

	if n, ok := cog.loadedFiles[file]; ok {
		return n, nil
	}

	cog.mux.Unlock()

	f, err := ParseFile(ctx, cog.Workspace, file)

	if err != nil {
		return nil, fmt.Errorf("failed to parse file (%v) %w", file, err)
	}

	cog.mux.Lock()
	cog.loadedFiles[file] = f

	cog.Graph.AddVertex(f)
	cog.mux.Unlock()

	return f, nil
}
