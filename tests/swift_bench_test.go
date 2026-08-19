package tests_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/languages"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

func countNodes(n ast.ASTNode) int {
	total := 1

	for _, child := range n.Children() {
		total += countNodes(child)
	}

	return total
}

// swiftStdlibInterface locates the Swift standard library's generated
// interface, which is the single file every stdlib definition resolves into.
// It is the worst case captn can be asked to parse for Swift.
func swiftStdlibInterface(t testing.TB) string {
	t.Helper()

	if runtime.GOOS != "darwin" {
		t.Skip("the Swift stdlib interface layout is probed through xcrun")
	}

	out, err := exec.Command("xcrun", "--show-sdk-path").Output()

	if err != nil {
		t.Skip("no Xcode SDK available")
	}

	pattern := filepath.Join(strings.TrimSpace(string(out)), "usr/lib/swift/Swift.swiftmodule/*-apple-macos.swiftinterface")
	matches, err := filepath.Glob(pattern)

	if err != nil || len(matches) == 0 {
		t.Skip("no Swift stdlib interface found")
	}

	return matches[0]
}

func goStdlibSource(t testing.TB) string {
	t.Helper()

	out, err := exec.Command("go", "env", "GOROOT").Output()

	if err != nil {
		t.Skip("no GOROOT available")
	}

	return filepath.Join(strings.TrimSpace(string(out)), "src", "fmt", "print.go")
}

func benchmarkParse(b *testing.B, path string, ext string) {
	buf, err := os.ReadFile(path)

	if err != nil {
		b.Skipf("cannot read %v: %v", path, err)
	}

	cwd, err := os.Getwd()

	if err != nil {
		b.Fatal(err)
	}

	src := common.NewSource(cwd, "bench"+ext, buf)

	b.SetBytes(int64(len(buf)))
	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := cog.ParseSource(context.Background(), src); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkParseSwiftStdlib(b *testing.B) {
	benchmarkParse(b, swiftStdlibInterface(b), ".swift")
}

func BenchmarkParseGoStdlib(b *testing.B) {
	benchmarkParse(b, goStdlibSource(b), ".go")
}

func BenchmarkParseSwiftFixture(b *testing.B) {
	benchmarkParse(b, "./fixtures/swift/multidep/Sources/App/main.swift", ".swift")
}

// samplePeakRSS tracks the high-water resident memory of this process until the
// returned stop function is called
func samplePeakRSS() func() uint64 {
	var peak atomic.Uint64
	stop := make(chan struct{})
	done := make(chan struct{})

	go func() {
		defer close(done)

		for {
			if v := rssBytes(); v > peak.Load() {
				peak.Store(v)
			}

			select {
			case <-stop:
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
	}()

	return func() uint64 {
		close(stop)
		<-done
		return peak.Load()
	}
}

// TestSwiftConcurrentPeakMemory measures what a fan-out of dependency loads
// costs at peak. LoadFiles resolves dependencies through common.ParallelCollect,
// which spawns one goroutine per file with no bound, so every concurrent parse
// holds its own tree-sitter tree at the same time.
func TestSwiftConcurrentPeakMemory(t *testing.T) {
	if os.Getenv("CAPTN_BENCH") == "" {
		t.Skip("set CAPTN_BENCH=1 to run the peak memory report")
	}

	if runtime.GOOS != "darwin" {
		t.Skip("the Swift SDK layout is probed through xcrun")
	}

	out, err := exec.Command("xcrun", "--show-sdk-path").Output()

	if err != nil {
		t.Skip("no Xcode SDK available")
	}

	matches, err := filepath.Glob(filepath.Join(strings.TrimSpace(string(out)), "usr/lib/swift/*.swiftmodule/*-apple-macos.swiftinterface"))

	if err != nil || len(matches) < 2 {
		t.Skip("not enough Swift interfaces to fan out over")
	}

	sort.Slice(matches, func(i, j int) bool {
		a, _ := os.Stat(matches[i])
		b, _ := os.Stat(matches[j])
		return a.Size() > b.Size()
	})

	fanout := 6

	if v, err := strconv.Atoi(os.Getenv("CAPTN_FANOUT")); err == nil && v > 0 {
		fanout = v
	}

	if len(matches) > fanout {
		matches = matches[:fanout]
	}

	cwd, err := os.Getwd()

	if err != nil {
		t.Fatal(err)
	}

	total := 0

	for _, p := range matches {
		if info, err := os.Stat(p); err == nil {
			total += int(info.Size())
		}
	}

	parse := func(ctx context.Context, i int, p string) (*cog.COGFile, error) {
		buf, err := os.ReadFile(p)

		if err != nil {
			return nil, err
		}

		return cog.ParseSource(ctx, common.NewSource(cwd, fmt.Sprintf("mod%d.swift", i), buf))
	}

	runtime.GC()
	baseline := rssBytes()

	// Sequential, for the floor
	stopSeq := samplePeakRSS()
	seqStart := time.Now()

	for i, p := range matches {
		if _, err := parse(context.Background(), i, p); err != nil {
			t.Fatal(err)
		}
	}

	seqElapsed := time.Since(seqStart)
	seqPeak := stopSeq()

	runtime.GC()

	// Concurrent, the way LoadFiles actually does it
	stopPar := samplePeakRSS()
	parStart := time.Now()

	indexed := make([]int, len(matches))

	for i := range matches {
		indexed[i] = i
	}

	if _, err := common.ParallelCollect(context.Background(), indexed, func(ctx context.Context, i int) (*cog.COGFile, error) {
		return parse(ctx, i, matches[i])
	}); err != nil {
		t.Fatal(err)
	}

	parElapsed := time.Since(parStart)
	parPeak := stopPar()

	mb := func(v uint64) string { return fmt.Sprintf("%.0f MB", float64(v)/(1<<20)) }

	fmt.Printf("\n%d Swift interfaces, %.2f MB of source total\n\n", len(matches), float64(total)/(1<<20))
	fmt.Printf("  baseline rss       : %s\n", mb(baseline))
	fmt.Printf("  sequential  peak   : %s  (+%s over baseline, %v)\n", mb(seqPeak), mb(seqPeak-baseline), seqElapsed.Round(time.Millisecond))
	fmt.Printf("  concurrent  peak   : %s  (+%s over baseline, %v)\n", mb(parPeak), mb(parPeak-baseline), parElapsed.Round(time.Millisecond))
	fmt.Println()
}

// rssBytes reads process resident memory, which unlike runtime.MemStats also
// counts the cgo allocations the tree-sitter tree lives in
func rssBytes() uint64 {
	out, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(os.Getpid())).Output()

	if err != nil {
		return 0
	}

	kb, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)

	if err != nil {
		return 0
	}

	return kb * 1024
}

// TestSwiftStdlibTreeSize measures what one concrete tree actually retains.
// A single before/after RSS reading cannot answer this: the first parse also
// pages in the grammar's static tables and raises the allocator's high-water
// mark with parse scratch, and freed C memory is not returned to the OS. Holding
// several trees and taking the marginal cost per tree isolates the tree itself.
func TestSwiftStdlibTreeSize(t *testing.T) {
	if os.Getenv("CAPTN_BENCH") == "" {
		t.Skip("set CAPTN_BENCH=1 to run the tree size report")
	}

	const held = 6

	path := swiftStdlibInterface(t)

	buf, err := os.ReadFile(path)

	if err != nil {
		t.Skipf("cannot read %v: %v", path, err)
	}

	tsp := tree_sitter.NewParser()
	defer tsp.Close()

	if err := tsp.SetLanguage(languages.Swift.GetTreeSitterLanguage()); err != nil {
		t.Fatal(err)
	}

	trees := make([]*tree_sitter.Tree, 0, held)

	// The first parse carries the one-time costs; measure from after it
	trees = append(trees, tsp.Parse(buf, nil))

	runtime.GC()
	baseline := rssBytes()

	start := time.Now()

	for i := 1; i < held; i++ {
		trees = append(trees, tsp.Parse(buf, nil))
	}

	elapsed := time.Since(start)

	runtime.GC()
	loaded := rssBytes()

	perTree := float64(loaded-baseline) / float64(held-1)

	fmt.Printf("\nswift stdlib interface: %d bytes (%.2f MB)\n", len(buf), float64(len(buf))/(1<<20))
	fmt.Printf("  trees held             : %d\n", held)
	fmt.Printf("  marginal rss per tree  : %.2f MB\n", perTree/(1<<20))
	fmt.Printf("  tree : source ratio    : %.2fx\n", perTree/float64(len(buf)))
	fmt.Printf("  steady-state parse     : %v per tree\n", (elapsed / time.Duration(held-1)).Round(time.Millisecond))
	fmt.Println()

	for _, tree := range trees {
		tree.Close()
	}
}

// TestSwiftStdlibFullBudget reports the whole cost of one file: the concrete
// tree on the C heap, captn's AST and index on the Go heap, and the time each
// phase takes.
func TestSwiftStdlibFullBudget(t *testing.T) {
	if os.Getenv("CAPTN_BENCH") == "" {
		t.Skip("set CAPTN_BENCH=1 to run the full budget report")
	}

	path := swiftStdlibInterface(t)

	cwd, err := os.Getwd()

	if err != nil {
		t.Fatal(err)
	}

	sample := func() (uint64, uint64) {
		var ms runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&ms)
		return ms.HeapAlloc, rssBytes()
	}

	heap0, rss0 := sample()

	buf, err := os.ReadFile(path)

	if err != nil {
		t.Skipf("cannot read %v: %v", path, err)
	}

	src := common.NewSource(cwd, "bench.swift", buf)
	heap1, rss1 := sample()

	tsp := tree_sitter.NewParser()

	if err := tsp.SetLanguage(languages.Swift.GetTreeSitterLanguage()); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	tree := tsp.Parse(buf, nil)
	parseTime := time.Since(start)
	heap2, rss2 := sample()

	start = time.Now()
	module, err := languages.Swift.Parse(context.Background(), src, tree)
	transformTime := time.Since(start)

	if err != nil {
		t.Fatal(err)
	}

	heap3, rss3 := sample()

	pf := cog.NewCOGFile(src, module, languages.Swift)

	start = time.Now()
	pf.IndexNodes()
	indexTime := time.Since(start)
	heap4, rss4 := sample()

	mb := func(a, b uint64) string {
		if a < b {
			return "         -"
		}
		return fmt.Sprintf("%7.2f MB", float64(a-b)/(1<<20))
	}

	fmt.Printf("\nswift stdlib interface: %d bytes, %d AST nodes\n\n", len(buf), countNodes(module))
	fmt.Printf("%-24s %12s %12s %14s\n", "phase", "time", "go heap", "process rss")
	fmt.Printf("%-24s %12s %12s %14s\n", "source buffer", "-", mb(heap1, heap0), mb(rss1, rss0))
	fmt.Printf("%-24s %12s %12s %14s\n", "tree-sitter tree (cgo)", parseTime.Round(time.Millisecond), mb(heap2, heap1), mb(rss2, rss1))
	fmt.Printf("%-24s %12s %12s %14s\n", "captn AST (transform)", transformTime.Round(time.Millisecond), mb(heap3, heap2), mb(rss3, rss2))
	fmt.Printf("%-24s %12s %12s %14s\n", "hash + interval index", indexTime.Round(time.Millisecond), mb(heap4, heap3), mb(rss4, rss3))
	fmt.Printf("%-24s %12s %12s %14s\n", "TOTAL", (parseTime + transformTime + indexTime).Round(time.Millisecond), mb(heap4, heap0), mb(rss4, rss0))

	tree.Close()
	tsp.Close()

	_, rss5 := sample()
	fmt.Printf("\nafter tree.Close(): rss released %s (cgo tree)\n\n", mb(rss4, rss5))

	runtime.KeepAlive(pf)
}

// TestSwiftStdlibRetainedSize separates what a parse keeps from what it churns
// through. Benchmark B/op reports every byte allocated, most of which is freed
// immediately, so it says nothing about how large a parsed file actually is.
func TestSwiftStdlibRetainedSize(t *testing.T) {
	if os.Getenv("CAPTN_BENCH") == "" {
		t.Skip("set CAPTN_BENCH=1 to run the retained size report")
	}

	path := swiftStdlibInterface(t)

	buf, err := os.ReadFile(path)

	if err != nil {
		t.Skipf("cannot read %v: %v", path, err)
	}

	cwd, err := os.Getwd()

	if err != nil {
		t.Fatal(err)
	}

	var before, after runtime.MemStats

	runtime.GC()
	runtime.ReadMemStats(&before)

	pf, err := cog.ParseSource(context.Background(), common.NewSource(cwd, "bench.swift", buf))

	if err != nil {
		t.Fatal(err)
	}

	runtime.GC()
	runtime.ReadMemStats(&after)

	nodes := countNodes(pf.Module)

	fmt.Printf("\nswift stdlib interface\n")
	fmt.Printf("  file bytes           : %12d\n", len(buf))
	fmt.Printf("  AST nodes            : %12d\n", nodes)
	fmt.Printf("  retained after GC    : %12d  (the parsed file, held live)\n", after.HeapAlloc-before.HeapAlloc)
	fmt.Printf("  allocation churn     : %12d  (what B/op reports)\n", after.TotalAlloc-before.TotalAlloc)
	fmt.Printf("  retained per node    : %12d\n", int(after.HeapAlloc-before.HeapAlloc)/nodes)
	fmt.Println()

	runtime.KeepAlive(pf)
}

// TestSwiftStdlibPhaseSplit attributes the cost of the worst case to the three
// phases ParseSource runs: tree-sitter, the language transformer, and indexing.
// It parses megabytes and only reports, so it is opt-in rather than part of the
// normal suite: CAPTN_BENCH=1 go test -run PhaseSplit ./tests
func TestSwiftStdlibPhaseSplit(t *testing.T) {
	if os.Getenv("CAPTN_BENCH") == "" {
		t.Skip("set CAPTN_BENCH=1 to run the parse phase report")
	}

	for _, target := range []struct {
		label string
		path  string
		lang  languages.LanguageSupport
	}{
		{"swift stdlib interface", swiftStdlibInterface(t), languages.Swift},
		{"go stdlib fmt/print.go", goStdlibSource(t), languages.Golang},
	} {
		buf, err := os.ReadFile(target.path)

		if err != nil {
			t.Skipf("cannot read %v: %v", target.path, err)
		}

		cwd, err := os.Getwd()

		if err != nil {
			t.Fatal(err)
		}

		src := common.NewSource(cwd, "bench", buf)

		tsp := tree_sitter.NewParser()

		if err := tsp.SetLanguage(target.lang.GetTreeSitterLanguage()); err != nil {
			t.Fatal(err)
		}

		start := time.Now()
		tree := tsp.Parse(buf, nil)
		parsed := time.Since(start)

		start = time.Now()
		module, err := target.lang.Parse(context.Background(), src, tree)
		transformed := time.Since(start)

		if err != nil {
			t.Fatal(err)
		}

		pf := cog.NewCOGFile(src, module, target.lang)

		start = time.Now()
		pf.IndexNodes()
		indexed := time.Since(start)

		tree.Close()
		tsp.Close()

		fmt.Printf("\n%s (%d bytes, %d nodes)\n", target.label, len(buf), countNodes(module))
		fmt.Printf("  tree-sitter parse : %v\n", parsed.Round(time.Microsecond))
		fmt.Printf("  transform         : %v\n", transformed.Round(time.Microsecond))
		fmt.Printf("  index + hash      : %v\n", indexed.Round(time.Microsecond))
	}

	fmt.Println()
}
