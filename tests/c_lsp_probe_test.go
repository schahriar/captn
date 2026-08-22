package tests_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/languages"
	"github.com/schahriar/captn/pkg/lsp"
	"github.com/stretchr/testify/assert"
)

func cCollectTypes(n ast.ASTNode, out *[]*ast.ASTTypeExpression) {
	if texpr, ok := n.(*ast.ASTTypeExpression); ok {
		*out = append(*out, texpr)
	}

	for _, child := range n.Children() {
		cCollectTypes(child, out)
	}
}

func cProbeFile(t *testing.T, ctx context.Context, client *lsp.Client, lang languages.LanguageSupport, cwd string, path string) {
	t.Helper()

	pf, err := cog.ParseFile(ctx, cwd, path)
	if !assert.NoError(t, err) {
		return
	}

	doc := lsp.NewTextDocumentItem(
		lsp.FileURI(pf.Source.Path),
		lang.GetLanguageID(),
		1,
		string(pf.Source.Buffer),
	)

	fmt.Printf("\n=== %v: imports ===\n", path)

	impVis := ast.NewImportVisitor()
	pf.Module.Accept(impVis)

	for _, imp := range impVis.Imports {
		name := "?"
		if imp.Reference != nil {
			name = imp.Reference.Name
		}

		var refs []lsp.Location

		// The background index may still be building; retry rather than
		// report a false negative
		for attempt := 0; attempt < 5; attempt++ {
			refs, err = client.ImportDefinition(ctx, doc, imp.GetPosition())

			if err == nil && len(refs) > 0 {
				break
			}

			time.Sleep(2 * time.Second)
		}

		if err != nil {
			fmt.Printf("  import %-16s ERROR %v\n", name, err)
			continue
		}

		if len(refs) == 0 {
			fmt.Printf("  import %-16s (no definition)\n", name)
			continue
		}

		for _, ref := range refs {
			fmt.Printf("  import %-16s -> %v %d:%d-%d:%d\n", name, ref.URI, ref.Range.Start.Line, ref.Range.Start.Character, ref.Range.End.Line, ref.Range.End.Character)
		}
	}

	fmt.Printf("\n=== %v: call expressions ===\n", path)

	var calls []*ast.ASTCallExpression
	collectCalls(pf.Module, &calls)

	for _, call := range calls {
		if call.Symbol == nil {
			continue
		}

		refs, err := client.Definition(ctx, lsp.DefinitionRequest{TextDocument: doc, Range: call.Symbol.GetPosition()})

		if err != nil {
			fmt.Printf("  call %-16s ERROR %v\n", call.Symbol.Name, err)
			continue
		}

		if len(refs) == 0 {
			fmt.Printf("  call %-16s (no definition)\n", call.Symbol.Name)
			continue
		}

		for _, ref := range refs {
			fmt.Printf("  call %-16s -> %v %d:%d-%d:%d\n", call.Symbol.Name, ref.URI, ref.Range.Start.Line, ref.Range.Start.Character, ref.Range.End.Line, ref.Range.End.Character)
		}
	}

	fmt.Printf("\n=== %v: type expressions ===\n", path)

	var types []*ast.ASTTypeExpression
	cCollectTypes(pf.Module, &types)

	for _, texpr := range types {
		refs, err := client.Definition(ctx, lsp.DefinitionRequest{TextDocument: doc, Range: texpr.GetPosition()})

		if err != nil {
			fmt.Printf("  type %-16s ERROR %v\n", texpr.Name, err)
			continue
		}

		if len(refs) == 0 {
			fmt.Printf("  type %-16s (no definition)\n", texpr.Name)
			continue
		}

		for _, ref := range refs {
			fmt.Printf("  type %-16s -> %v %d:%d-%d:%d\n", texpr.Name, ref.URI, ref.Range.Start.Line, ref.Range.Start.Character, ref.Range.End.Line, ref.Range.End.Character)
		}
	}

	fmt.Println()
}

func cFindFunctions(n ast.ASTNode, name string, out *[]*ast.ASTFuncExpression) {
	if fn, ok := n.(*ast.ASTFuncExpression); ok && fn.Name != nil && fn.Name.Name == name {
		*out = append(*out, fn)
	}

	for _, child := range n.Children() {
		cFindFunctions(child, name, out)
	}
}

func cAskDefinitionAt(t *testing.T, client *lsp.Client, lang languages.LanguageSupport, pf *cog.COGFile, fnName string, nth int, label string) {
	t.Helper()

	var fns []*ast.ASTFuncExpression
	cFindFunctions(pf.Module, fnName, &fns)

	if !assert.Greater(t, len(fns), nth, "expected to find %v in %v", fnName, pf.Source.Path) {
		return
	}

	doc := lsp.NewTextDocumentItem(
		lsp.FileURI(pf.Source.Path),
		lang.GetLanguageID(),
		1,
		string(pf.Source.Buffer),
	)

	refs, err := client.Definition(t.Context(), lsp.DefinitionRequest{TextDocument: doc, Range: fns[nth].Name.GetPosition()})

	if err != nil {
		fmt.Printf("  %-34s ERROR %v\n", label, err)
		return
	}

	if len(refs) == 0 {
		fmt.Printf("  %-34s (no definition)\n", label)
		return
	}

	for _, ref := range refs {
		fmt.Printf("  %-34s -> %v %d:%d-%d:%d\n", label, ref.URI, ref.Range.Start.Line, ref.Range.Start.Character, ref.Range.End.Line, ref.Range.End.Character)
	}
}

// TestCDeclDefProbe reports whether clangd can hop from a header prototype to
// its implementation. The hop rides clangd's index, which knows files in
// three tiers: nothing (cold), files didOpen'd this session, and everything
// enumerable through compile_commands.json via the background index.
func TestCDeclDefProbe(t *testing.T) {
	if os.Getenv("CAPTN_PROBE") == "" {
		t.Skip("set CAPTN_PROBE=1 to probe clangd")
	}

	ctx := t.Context()

	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	header := parseTestFile(t, "./fixtures/c/multidep/widget.h")
	impl := parseTestFile(t, "./fixtures/c/multidep/widget.c")
	hpp := parseTestFile(t, "./fixtures/cpp/multidep/gadget.hpp")
	cpp := parseTestFile(t, "./fixtures/cpp/multidep/gadget.cpp")

	opts := lsp.NewStartOptions(cwd, "captn-probe", "0.1.0", languages.C.NewLSPServer)

	client, err := lsp.Start(ctx, opts)
	if !assert.NoError(t, err) {
		return
	}

	fmt.Println("\n=== tier 1: cold session, header only ===")
	cAskDefinitionAt(t, client, languages.CPP, header, "widget_make", 0, "widget.h proto (cold)")
	cAskDefinitionAt(t, client, languages.CPP, hpp, "describe", 0, "gadget.hpp proto (cold)")

	fmt.Println("\n=== tier 2: after the implementation file is opened ===")
	cAskDefinitionAt(t, client, languages.C, impl, "widget_make", 0, "widget.c def (toggle check)")
	cAskDefinitionAt(t, client, languages.CPP, cpp, "describe", 0, "gadget.cpp def (toggle check)")
	cAskDefinitionAt(t, client, languages.CPP, header, "widget_make", 0, "widget.h proto (impl opened)")
	cAskDefinitionAt(t, client, languages.CPP, hpp, "describe", 0, "gadget.hpp proto (impl opened)")

	client.Close(ctx)

	fmt.Println("\n=== tier 3: fresh session with compile_commands.json ===")

	cdir := filepath.Join(cwd, "fixtures", "c", "multidep")
	cppdir := filepath.Join(cwd, "fixtures", "cpp", "multidep")

	writeDB := func(dir string, entries string) string {
		path := filepath.Join(dir, "compile_commands.json")
		assert.NoError(t, os.WriteFile(path, []byte(entries), 0o644))
		return path
	}

	db1 := writeDB(cdir, `[
	{"directory": "`+cdir+`", "command": "cc -c main.c", "file": "main.c"},
	{"directory": "`+cdir+`", "command": "cc -c widget.c", "file": "widget.c"}
]`)
	db2 := writeDB(cppdir, `[
	{"directory": "`+cppdir+`", "command": "c++ -std=c++17 -c main.cpp", "file": "main.cpp"},
	{"directory": "`+cppdir+`", "command": "c++ -std=c++17 -c gadget.cpp", "file": "gadget.cpp"}
]`)

	defer func() {
		os.Remove(db1)
		os.Remove(db2)
		os.RemoveAll(filepath.Join(cdir, ".cache"))
		os.RemoveAll(filepath.Join(cppdir, ".cache"))
	}()

	client2, err := lsp.Start(ctx, opts)
	if !assert.NoError(t, err) {
		return
	}

	defer client2.Close(ctx)

	// The background index builds asynchronously after startup
	for attempt := 0; attempt < 5; attempt++ {
		time.Sleep(2 * time.Second)
		fmt.Printf("--- attempt %d\n", attempt+1)
		cAskDefinitionAt(t, client2, languages.CPP, header, "widget_make", 0, "widget.h proto (bg index)")
		cAskDefinitionAt(t, client2, languages.CPP, hpp, "describe", 0, "gadget.hpp proto (bg index)")
	}
}

// TestCLSPProbe reports the raw locations clangd returns, before captn
// interprets them. What comes back here decides how ClassifyImportType and
// NormalizeDefinitionRange must be written.
func TestCLSPProbe(t *testing.T) {
	if os.Getenv("CAPTN_PROBE") == "" {
		t.Skip("set CAPTN_PROBE=1 to probe clangd")
	}

	ctx := t.Context()

	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	opts := lsp.NewStartOptions(cwd, "captn-probe", "0.1.0", languages.C.NewLSPServer)
	opts.InitializationOptions = languages.C.GetLSPInitializationOptions(ctx, cwd)

	client, err := lsp.Start(ctx, opts)
	if !assert.NoError(t, err) {
		return
	}

	defer client.Close(ctx)

	cProbeFile(t, ctx, client, languages.C, cwd, "./fixtures/c/multidep/main.c")
	cProbeFile(t, ctx, client, languages.CPP, cwd, "./fixtures/cpp/multidep/main.cpp")
	cProbeFile(t, ctx, client, languages.CPP, cwd, "./fixtures/cpp/baseproj/types.cpp")
}
