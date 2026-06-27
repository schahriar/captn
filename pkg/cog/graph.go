package cog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/dominikbraun/graph"
	"github.com/gofrs/flock"
	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/cgraph"
	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/lsp"
)

var cogCache = map[string]*COG{}
var cogCacheMux = &sync.Mutex{}

type COG struct {
	loadedFiles  map[string]*COGFile      `json:"-"`
	loadingFiles map[string]*loadFileCall `json:"-"`

	Mux *sync.Mutex `json:"-"`

	Graph            graph.Graph[common.HashType, COGNode] `json:"-"`
	Workspace        string
	ObservationCache map[common.HashType]common.ObservationSchema
}

type loadFileCall struct {
	done chan struct{}
	file *COGFile
	err  error
}

func IsNodeOfInterest(a ast.ASTNode) bool {
	switch a.(type) {
	case *ast.ASTFuncExpression:
		return true
	case *ast.ASTModule:
		return true // Catch all parent
	default:
		return false
	}
}

func newLoadFileCall() *loadFileCall {
	return &loadFileCall{
		done: make(chan struct{}),
	}
}

// COGNode - COG has polymorphic nodes as long as they conform to this interface
type COGNode interface {
	GetHash() common.HashType
	GetFilePath() string
	GetLanguage() string
	GetStringSource() string
}

func NodeHasher(cogn COGNode) common.HashType {
	return cogn.GetHash()
}

func NewCOG(workspace string) *COG {
	return &COG{
		Graph:            graph.New(NodeHasher),
		Mux:              &sync.Mutex{},
		loadedFiles:      map[string]*COGFile{},
		loadingFiles:     map[string]*loadFileCall{},
		ObservationCache: map[common.HashType]common.ObservationSchema{},
		Workspace:        workspace,
	}
}

func (cog *COG) FilePath() string {
	return filepath.Join(cog.Workspace, "captn.cog")
}

func OpenCOG(workspace string) (*COG, error) {
	cogCacheMux.Lock()
	if cog, ok := cogCache[workspace]; ok {
		cogCacheMux.Unlock()
		return cog, nil
	}

	cogCacheMux.Unlock()

	cog := NewCOG(workspace)

	// Important: Lock file first to avoid deadlock
	fl, err := cog.LockFile()

	if err != nil {
		return nil, err
	}

	defer fl.Unlock()

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

	cog.Mux.Lock()
	err = cog.unmarshalLocked(b)
	cog.Mux.Unlock()

	if err != nil {
		return nil, fmt.Errorf("Failed to decode COG at workspace %v with error %w", workspace, err)
	}

	cogCacheMux.Lock()
	cogCache[workspace] = cog
	cogCacheMux.Unlock()

	return cog, nil
}

func (cog *COG) Marshal() ([]byte, error) {
	cog.Mux.Lock()
	defer cog.Mux.Unlock()

	return cog.marshalLocked()
}

func (cog *COG) marshalLocked() ([]byte, error) {
	var builder strings.Builder

	for h, f := range cog.ObservationCache {
		v, err := f.Marshal()

		if err != nil {
			return nil, fmt.Errorf("failed to serialize observation %v with error %w", h.String(), err)
		}

		fmt.Fprintf(&builder, "%s = %s\n", h.String(), v)
	}

	return []byte(builder.String()), nil
}

func (cog *COG) LockFile() (*flock.Flock, error) {
	cog.Mux.Lock()
	defer cog.Mux.Unlock()
	fileLock := flock.New(cog.FilePath())
	locked, err := fileLock.TryLock()

	if err != nil {
		return nil, fmt.Errorf("failed to acquire file lock %w", err)
	}
	if !locked {
		return nil, fmt.Errorf("file is already locked")
	}

	return fileLock, nil
}

func (cog *COG) Unmarshal(data []byte) error {
	// Important: Lock file first to avoid deadlock
	fl, err := cog.LockFile()

	if err != nil {
		return err
	}

	defer fl.Unlock()

	cog.Mux.Lock()
	defer cog.Mux.Unlock()

	return cog.unmarshalLocked(data)
}

func (cog *COG) unmarshalLocked(data []byte) error {
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		parts := strings.SplitN(line, " = ", 2)

		if len(parts) != 2 {
			return fmt.Errorf("failed to parse observation line %v", line)
		}

		h, err := common.UnmarshalHashType([]byte(strings.TrimSpace(parts[0])))

		if err != nil {
			return fmt.Errorf("failed to parse observation hash %v with error %w", parts[0], err)
		}

		v, err := common.UnmarshalObservationSchema([]byte(strings.TrimSpace(parts[1])))

		if err != nil {
			return fmt.Errorf("failed to parse observation value %v with error %w", parts[1], err)
		}

		cog.ObservationCache[*h] = v
	}

	return nil
}

func (cog *COG) Persist() error {
	cog.Mux.Lock()
	defer cog.Mux.Unlock()

	b, err := cog.marshalLocked()

	if err != nil {
		return err
	}

	return os.WriteFile(cog.FilePath(), b, 0644)
}

func (cog *COG) LoadFile(ctx context.Context, file string) (*COGFile, error) {
	cog.Mux.Lock()
	if cog.loadedFiles == nil {
		cog.loadedFiles = map[string]*COGFile{}
	}
	if cog.loadingFiles == nil {
		cog.loadingFiles = map[string]*loadFileCall{}
	}

	if n, ok := cog.loadedFiles[file]; ok {
		cog.Mux.Unlock()
		return n, nil
	}

	if call, ok := cog.loadingFiles[file]; ok {
		cog.Mux.Unlock()

		select {
		case <-call.done:
			return call.file, call.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	call := newLoadFileCall()
	cog.loadingFiles[file] = call
	cog.Mux.Unlock()

	defer func() {
		close(call.done)

		cog.Mux.Lock()
		delete(cog.loadingFiles, file)
		cog.Mux.Unlock()
	}()

	f, err := ParseFile(ctx, cog.Workspace, file)

	if err != nil {
		call.err = fmt.Errorf("failed to parse file (%v) %w", file, err)
		return nil, call.err
	}

	cog.Mux.Lock()
	if err := cog.Graph.AddVertex(f, graph.VertexAttribute("import_type", string(f.Language.ClassifyImportType(f.Source)))); err != nil {
		cog.Mux.Unlock()
		call.err = err
		return nil, call.err
	}
	cog.loadedFiles[file] = f
	cog.Mux.Unlock()

	call.file = f
	return f, nil
}

func (cog *COG) LoadFiles(ctx context.Context, files []string) ([]*COGFile, error) {
	return common.ParallelCollect(ctx, files, func(ctx context.Context, file string) (*COGFile, error) {
		return cog.LoadFile(ctx, file)
	})
}

func (cog *COG) QuerySnippet(ctx context.Context, file string, snippet string) (*ObservationGraph, COGNode, error) {
	f, err := cog.LoadFile(ctx, file)

	if err != nil {
		return nil, nil, err
	}

	r, err := f.FindSnippetRange([]byte(snippet))

	if err != nil {
		return nil, nil, err
	}

	chlds := f.QueryNodesWithinRange(r)
	root := f.Module

	defs := map[ast.ASTNode]*common.FileRange{}

	// TODO: We may need to breakdown refs by language for cross-language references
	client, err := loadLSPServerForLanguage(f.Language, f.Source.Workspace)

	if err != nil {
		return nil, nil, err
	}

	// Explode by nodes of interest
	for _, chld := range chlds {
		switch v := chld.(type) {
		case *ast.ASTCallExpression:
			defs[v] = v.Symbol.GetPosition()
			break
		}
	}

	// Batch load definitions
	inodes, err := client.Definitions(ctx, lsp.NewDefinitionBatchRequest(
		lsp.NewTextDocumentItem(
			lsp.FileURI(f.Source.Path),
			f.Language.GetLanguageID(),
			1,
			string(f.Source.Buffer),
		),
		common.ValuesOf(defs),
	))

	if err != nil {
		return nil, nil, fmt.Errorf("failed to get nodes of interest definitions with error %w", err)
	}

	deps := common.ResolvedDependencies{}

	for internalRange, refs := range inodes {
		// Note that definitions can return multiple locations for one range
		// This isn't a common use-case in most languages but added for future compatibility
		for _, ref := range refs {
			dep, err := NewResolvedDependencyFromURIFromCOGFile(ctx, f, internalRange, ref)

			if err != nil {
				return nil, nil, fmt.Errorf("failed to resolve dependency at %v with error %w", fmt.Sprintf("%v:%v:%v", ref.URI, ref.Range.Start.Line, ref.Range.Start.Character), err)
			}

			deps = append(deps, dep)
		}
	}

	// Preload dependencies
	if _, err = cog.LoadFiles(ctx, common.Map(deps, func(dep common.ResolvedDependency) string {
		return dep.External.Source.Path
	})); err != nil {
		return nil, nil, err
	}

	ichains := map[ast.ASTNode]ast.ASTNode{}

	// Now that we have preloaded and parsed the files we can efficiently find relevant nodes at depth=1
	/*
		We generate 2 things in this pass:
			1. Additional nodes added to chlds for collation (two-way interval lookup in the source / target files)
			2. Linkage between a symbol and an imported symbol (important for edges)
	*/
	for _, dep := range deps {
		// First resolve the source of the dependency in the current file
		nodes := f.QueryNodesWithinRange(dep.Internal)

		if len(nodes) != 1 {
			return nil, nil, fmt.Errorf("expected 1 node for dependency %v, received %v", dep.Internal, len(nodes))
		}

		inode := nodes[0]

		// This load file is going to use the cache since we prefetched
		ef, err := cog.LoadFile(ctx, dep.External.Source.Path)

		if err != nil {
			return nil, nil, err
		}

		enodes := ef.QueryNodesWithinRange(dep.External)

		// Again we blindly expect the nodes to map correctly as dependency resolution should've worked correctly
		if len(enodes) != 1 {
			return nil, nil, fmt.Errorf("expected 1 node for dependent node %v as referenced by %v, received %v", dep.External, dep.Internal, len(enodes))
		}

		enode := enodes[0]

		chlds = append(chlds, enode)

		// Create linkage, effectively edges in the graph
		// For linkage we explicitly lookup nearest NOI
		noi := inode.NearestOrSelf(IsNodeOfInterest)
		ichains[enode] = noi
	}

	// Collate by blocks
	interests := map[ast.ASTNode]bool{} // Basically a Set
	for _, chld := range chlds {
		v := chld.NearestOrSelf(IsNodeOfInterest)

		if v != nil {
			interests[v] = true
		}
	}

	g := cgraph.NewGraph(NodeHasher)
	og := NewObservationGraph(&g)

	og.Graph.AddVertex(root)

	for n := range interests {
		og.Graph.AddVertex(n)
		// If the node exists in a chain then map it to its upstream
		// Otherwise map to root
		if sib, ok := ichains[n]; ok {
			og.Graph.AddEdge(ast.GetHash(sib), ast.GetHash(n))
		} else {
			og.Graph.AddEdge(ast.GetHash(root), ast.GetHash(n))
		}
	}

	return og, root, nil
}

func (cog *COG) queryWithDepth(ctx context.Context, g *ObservationGraph, n COGNode, depth int, mux *sync.Mutex, visited map[common.HashType]bool) error {
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
	g := cgraph.NewGraph(NodeHasher)
	og := NewObservationGraph(&g)
	return og, cog.queryWithDepth(ctx, og, n, depth, &sync.Mutex{}, map[common.HashType]bool{})
}
