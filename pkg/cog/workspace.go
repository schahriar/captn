package cog

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/dominikbraun/graph"
	"github.com/gofrs/flock"
	"github.com/hashicorp/go-msgpack/v2/codec"
	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/cgraph"
	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/lsp"
)

var wspaceCache = map[string]*Workspace{}
var wspaceCacheMux = &sync.Mutex{}

type COGObservationMetadata struct {
	Anchors []*common.FileRange
}

func NewCOGObservationMetadata(anchors []*common.FileRange) COGObservationMetadata {
	return COGObservationMetadata{
		Anchors: anchors,
	}
}

type COGObservation struct {
	Answer   common.ObservationSchema
	Metadata COGObservationMetadata
}

func NewCOGObservation(ans common.ObservationSchema, anchors []*common.FileRange) COGObservation {
	return COGObservation{
		Answer:   ans,
		Metadata: NewCOGObservationMetadata(anchors),
	}
}

func (o COGObservation) Serialize() string {
	return o.Answer.Answer
}

type Workspace struct {
	loadedFiles  map[string]*COGFile      `json:"-"`
	loadingFiles map[string]*loadFileCall `json:"-"`

	Mux *sync.Mutex `json:"-"`

	Path             string
	ObservationCache map[common.HashType]COGObservation

	observationOrder map[common.HashType]int
	observationCount int
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
	GetFileRange() *common.FileRange
	GetLanguage() string
	GetStringSource() string
}

func NodeHasher(cogn COGNode) common.HashType {
	return cogn.GetHash()
}

func NewWorkspace(workspace string) *Workspace {
	return &Workspace{
		Mux:              &sync.Mutex{},
		loadedFiles:      map[string]*COGFile{},
		loadingFiles:     map[string]*loadFileCall{},
		ObservationCache: map[common.HashType]COGObservation{},
		observationOrder: map[common.HashType]int{},
		Path:             workspace,
	}
}

// SetObservationLocked stores an observation and assigns it the next write
// index if it is new. The caller must hold wspace.Mux.
func (wspace *Workspace) SetObservationLocked(h common.HashType, o COGObservation) {
	if wspace.observationOrder == nil {
		wspace.observationOrder = map[common.HashType]int{}
	}

	if _, ok := wspace.observationOrder[h]; !ok {
		wspace.observationOrder[h] = wspace.observationCount
		wspace.observationCount++
	}

	wspace.ObservationCache[h] = o
}

func (wspace *Workspace) SetObservation(h common.HashType, o COGObservation) {
	wspace.Mux.Lock()
	defer wspace.Mux.Unlock()

	wspace.SetObservationLocked(h, o)
}

func (wspace *Workspace) FilePath() string {
	return filepath.Join(wspace.Path, "captn.cog")
}

func OpenWorkspace(workspace string) (*Workspace, error) {
	wspaceCacheMux.Lock()
	if cog, ok := wspaceCache[workspace]; ok {
		wspaceCacheMux.Unlock()
		return cog, nil
	}

	wspaceCacheMux.Unlock()

	wspace := NewWorkspace(workspace)

	// Important: Lock file first to avoid deadlock
	fl, err := wspace.LockFile()

	if err != nil {
		return nil, err
	}

	defer fl.Unlock()

	f, err := os.OpenFile(wspace.FilePath(), os.O_RDWR, 0644)

	if errors.Is(err, os.ErrNotExist) {
		return wspace, nil
	} else if err != nil {
		return nil, fmt.Errorf("OpenCOG File Error %w", err)
	}

	defer f.Close()

	b, err := io.ReadAll(f)

	if err != nil {
		return nil, fmt.Errorf("OpenCOG File Read Error %w", err)
	}

	wspace.Mux.Lock()
	err = wspace.unmarshalLocked(b)
	wspace.Mux.Unlock()

	if err != nil {
		return nil, fmt.Errorf("Failed to decode COG at workspace %v with error %w", workspace, err)
	}

	wspaceCacheMux.Lock()
	wspaceCache[workspace] = wspace
	wspaceCacheMux.Unlock()

	return wspace, nil
}

func (wspace *Workspace) Marshal() ([]byte, error) {
	wspace.Mux.Lock()
	defer wspace.Mux.Unlock()

	return wspace.marshalLocked()
}

// observationMetadataWire is the messagepack schema for observation metadata.
// It encodes as a map so fields can be added without breaking older readers.
// AnswerLength is the byte length of the answer segment; answers are consumed
// by length so they can safely contain newlines.
type observationMetadataWire struct {
	Anchors      []string `codec:"anchors"`
	AnswerLength int      `codec:"answerLength"`
}

func encodeMetadata(m COGObservationMetadata, answerLength int) (string, error) {
	// banstructlit:ignore
	wire := observationMetadataWire{
		Anchors:      make([]string, 0, len(m.Anchors)),
		AnswerLength: answerLength,
	}

	for _, a := range m.Anchors {
		wire.Anchors = append(wire.Anchors, a.String())
	}

	var buf []byte

	if err := codec.NewEncoderBytes(&buf, &codec.MsgpackHandle{}).Encode(wire); err != nil {
		return "", err
	}

	return fmt.Sprintf("%d:%s", len(buf), hex.EncodeToString(buf)), nil
}

// decodeMetadata parses the metadata segment and returns the metadata, the
// answer byte length and the remainder of the data starting at the answer.
// The length header is used to skip past the hex payload rather than scanning
// for a separator.
func decodeMetadata(workspace string, segment string) (COGObservationMetadata, int, string, error) {
	meta := NewCOGObservationMetadata(nil)
	header, body, ok := strings.Cut(segment, ":")

	if !ok {
		return meta, 0, "", fmt.Errorf("missing metadata length header")
	}

	msgLen, err := strconv.Atoi(header)

	if err != nil || msgLen < 0 {
		return meta, 0, "", fmt.Errorf("invalid metadata length header %q", header)
	}

	hexLen := msgLen * 2

	if len(body) < hexLen+1 || body[hexLen] != ' ' {
		return meta, 0, "", fmt.Errorf("metadata segment is truncated or missing separator")
	}

	buf, err := hex.DecodeString(body[:hexLen])

	if err != nil {
		return meta, 0, "", fmt.Errorf("failed to decode metadata hex with error %w", err)
	}

	var wire observationMetadataWire

	if err := codec.NewDecoderBytes(buf, &codec.MsgpackHandle{}).Decode(&wire); err != nil {
		return meta, 0, "", fmt.Errorf("failed to decode metadata messagepack with error %w", err)
	}

	if wire.AnswerLength < 0 {
		return meta, 0, "", fmt.Errorf("invalid answer length %v", wire.AnswerLength)
	}

	meta.Anchors = make([]*common.FileRange, 0, len(wire.Anchors))

	for _, s := range wire.Anchors {
		fr, err := common.UnmarshalFileRange(workspace, s)

		if err != nil {
			return meta, 0, "", err
		}

		meta.Anchors = append(meta.Anchors, fr)
	}

	return meta, wire.AnswerLength, body[hexLen+1:], nil
}

func (wspace *Workspace) marshalLocked() ([]byte, error) {
	var builder strings.Builder

	// Observations written to the cache directly rather than through
	// SetObservation have no index yet; assign them deterministically
	keys := make([]common.HashType, 0, len(wspace.ObservationCache))
	unindexed := []common.HashType{}

	for h := range wspace.ObservationCache {
		keys = append(keys, h)

		if _, ok := wspace.observationOrder[h]; !ok {
			unindexed = append(unindexed, h)
		}
	}

	sort.Slice(unindexed, func(i, j int) bool {
		return unindexed[i].String() < unindexed[j].String()
	})

	for _, h := range unindexed {
		wspace.SetObservationLocked(h, wspace.ObservationCache[h])
	}

	sort.Slice(keys, func(i, j int) bool {
		return wspace.observationOrder[keys[i]] < wspace.observationOrder[keys[j]]
	})

	for _, h := range keys {
		o := wspace.ObservationCache[h]
		answer := o.Serialize()
		meta, err := encodeMetadata(o.Metadata, len(answer))

		if err != nil {
			return nil, fmt.Errorf("failed to encode metadata for observation %v with error %w", h.String(), err)
		}

		fmt.Fprintf(&builder, "%s %s %s\n", h.String(), meta, answer)
	}

	return []byte(builder.String()), nil
}

func (wspace *Workspace) LockFile() (*flock.Flock, error) {
	wspace.Mux.Lock()
	defer wspace.Mux.Unlock()
	fileLock := flock.New(wspace.FilePath())
	locked, err := fileLock.TryLock()

	if err != nil {
		return nil, fmt.Errorf("failed to acquire file lock %w", err)
	}
	if !locked {
		return nil, fmt.Errorf("file is already locked")
	}

	return fileLock, nil
}

func (wspace *Workspace) Unmarshal(data []byte) error {
	// Important: Lock file first to avoid deadlock
	fl, err := wspace.LockFile()

	if err != nil {
		return err
	}

	defer fl.Unlock()

	wspace.Mux.Lock()
	defer wspace.Mux.Unlock()

	return wspace.unmarshalLocked(data)
}

// unmarshalLocked reads observations sequentially rather than line by line;
// answers are consumed by the length recorded in the metadata so they can
// contain newlines.
func (wspace *Workspace) unmarshalLocked(data []byte) error {
	rest := string(data)

	for strings.TrimSpace(rest) != "" {
		rest = strings.TrimLeft(rest, "\n")

		hashPart, tail, ok := strings.Cut(rest, " ")

		if !ok {
			return fmt.Errorf("failed to parse observation at %q", rest)
		}

		h, err := common.UnmarshalHashType([]byte(hashPart))

		if err != nil {
			return fmt.Errorf("failed to parse observation hash %v with error %w", hashPart, err)
		}

		meta, answerLen, tail, err := decodeMetadata(wspace.Path, tail)

		if err != nil {
			return fmt.Errorf("failed to parse observation metadata for %v with error %w", hashPart, err)
		}

		if len(tail) < answerLen {
			return fmt.Errorf("truncated answer for observation %v", hashPart)
		}

		answer := tail[:answerLen]
		rest = tail[answerLen:]

		if len(rest) > 0 && rest[0] != '\n' {
			return fmt.Errorf("missing record separator after observation %v", hashPart)
		}

		wspace.SetObservationLocked(*h, NewCOGObservation(common.NewObservationSchema(hashPart, answer), meta.Anchors))
	}

	return nil
}

func (wspace *Workspace) Persist() error {
	wspace.Mux.Lock()
	defer wspace.Mux.Unlock()

	b, err := wspace.marshalLocked()

	if err != nil {
		return err
	}

	return os.WriteFile(wspace.FilePath(), b, 0644)
}

func (wspace *Workspace) LoadFile(ctx context.Context, file string) (*COGFile, error) {
	wspace.Mux.Lock()
	if wspace.loadedFiles == nil {
		wspace.loadedFiles = map[string]*COGFile{}
	}
	if wspace.loadingFiles == nil {
		wspace.loadingFiles = map[string]*loadFileCall{}
	}

	if n, ok := wspace.loadedFiles[file]; ok {
		wspace.Mux.Unlock()
		return n, nil
	}

	if call, ok := wspace.loadingFiles[file]; ok {
		wspace.Mux.Unlock()

		select {
		case <-call.done:
			return call.file, call.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	call := newLoadFileCall()
	wspace.loadingFiles[file] = call
	wspace.Mux.Unlock()

	defer func() {
		close(call.done)

		wspace.Mux.Lock()
		delete(wspace.loadingFiles, file)
		wspace.Mux.Unlock()
	}()

	f, err := ParseFile(ctx, wspace.Path, file)

	if err != nil {
		call.err = fmt.Errorf("failed to parse file (%v) %w", file, err)
		return nil, call.err
	}

	wspace.Mux.Lock()
	wspace.loadedFiles[file] = f
	wspace.Mux.Unlock()

	call.file = f
	return f, nil
}

func (wspace *Workspace) LoadFiles(ctx context.Context, files []string) ([]*COGFile, error) {
	return common.ParallelCollect(ctx, files, func(ctx context.Context, file string) (*COGFile, error) {
		return wspace.LoadFile(ctx, file)
	})
}

func (wspace *Workspace) SearchSnippet(ctx context.Context, file string, snippet string) (*ObservationGraph, COGNode, error) {
	f, err := wspace.LoadFile(ctx, file)

	if err != nil {
		return nil, nil, err
	}

	r, err := f.FindSnippetRange([]byte(snippet))

	if err != nil {
		return nil, nil, err
	}

	chlds := f.FindNodesWithinRange(r)
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
	if _, err = wspace.LoadFiles(ctx, common.Map(deps, func(dep common.ResolvedDependency) string {
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
		nodes := f.FindNodesWithinRange(dep.Internal)

		if len(nodes) != 1 {
			return nil, nil, fmt.Errorf("expected 1 node for dependency %v, received %v", dep.Internal, len(nodes))
		}

		inode := nodes[0]

		// This load file is going to use the cache since we prefetched
		ef, err := wspace.LoadFile(ctx, dep.External.Source.Path)

		if err != nil {
			return nil, nil, err
		}

		enodes := ef.FindNodesWithinRange(dep.External)

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

func (wspace *Workspace) queryWithDepth(ctx context.Context, g *ObservationGraph, n COGNode, depth int, mux *sync.Mutex, visited map[common.HashType]bool) error {
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
				f, err := wspace.LoadFile(ctx, imp.External.Source.Path)

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

				if err := wspace.queryWithDepth(ctx, g, f, depth-1, mux, visited); err != nil {
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

func (wspace *Workspace) QueryWithDepth(ctx context.Context, n COGNode, depth int) (*ObservationGraph, error) {
	g := cgraph.NewGraph(NodeHasher)
	og := NewObservationGraph(&g)
	return og, wspace.queryWithDepth(ctx, og, n, depth, &sync.Mutex{}, map[common.HashType]bool{})
}
