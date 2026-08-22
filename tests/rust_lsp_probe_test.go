package tests_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/languages"
	"github.com/schahriar/captn/pkg/lsp"
	"github.com/stretchr/testify/assert"
)

func rustCollectTypes(n ast.ASTNode, out *[]*ast.ASTTypeExpression) {
	if texpr, ok := n.(*ast.ASTTypeExpression); ok {
		*out = append(*out, texpr)
	}

	for _, child := range n.Children() {
		rustCollectTypes(child, out)
	}
}

func rustProbeFile(t *testing.T, ctx context.Context, client *lsp.Client, cwd string, path string) {
	t.Helper()

	pf, err := cog.ParseFile(ctx, cwd, path)
	if !assert.NoError(t, err) {
		return
	}

	doc := lsp.NewTextDocumentItem(
		lsp.FileURI(pf.Source.Path),
		languages.Rust.GetLanguageID(),
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

		// cargo metadata may still be loading; retry rather than report a
		// false negative
		for attempt := 0; attempt < 5; attempt++ {
			refs, err = client.ImportDefinition(ctx, doc, imp.GetPosition())

			if err == nil && len(refs) > 0 {
				break
			}

			time.Sleep(2 * time.Second)
		}

		if err != nil {
			fmt.Printf("  import %-28s ERROR %v\n", name, err)
			continue
		}

		if len(refs) == 0 {
			fmt.Printf("  import %-28s (no definition)\n", name)
			continue
		}

		for _, ref := range refs {
			fmt.Printf("  import %-28s -> %v %d:%d-%d:%d\n", name, ref.URI, ref.Range.Start.Line, ref.Range.Start.Character, ref.Range.End.Line, ref.Range.End.Character)
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
			fmt.Printf("  call %-28s ERROR %v\n", call.Symbol.Name, err)
			continue
		}

		if len(refs) == 0 {
			fmt.Printf("  call %-28s (no definition)\n", call.Symbol.Name)
			continue
		}

		for _, ref := range refs {
			fmt.Printf("  call %-28s -> %v %d:%d-%d:%d\n", call.Symbol.Name, ref.URI, ref.Range.Start.Line, ref.Range.Start.Character, ref.Range.End.Line, ref.Range.End.Character)
		}
	}

	fmt.Printf("\n=== %v: type expressions ===\n", path)

	var types []*ast.ASTTypeExpression
	rustCollectTypes(pf.Module, &types)

	for _, texpr := range types {
		refs, err := client.Definition(ctx, lsp.DefinitionRequest{TextDocument: doc, Range: texpr.GetPosition()})

		if err != nil {
			fmt.Printf("  type %-28s ERROR %v\n", texpr.Name, err)
			continue
		}

		if len(refs) == 0 {
			fmt.Printf("  type %-28s (no definition)\n", texpr.Name)
			continue
		}

		for _, ref := range refs {
			fmt.Printf("  type %-28s -> %v %d:%d-%d:%d\n", texpr.Name, ref.URI, ref.Range.Start.Line, ref.Range.Start.Character, ref.Range.End.Line, ref.Range.End.Character)
		}
	}

	fmt.Println()
}

// TestRustLSPProbe reports the raw locations rust-analyzer returns, before
// captn interprets them. Run it when deciding what ClassifyImportType and
// NormalizeDefinitionRange must handle.
func TestRustLSPProbe(t *testing.T) {
	if os.Getenv("CAPTN_PROBE") == "" {
		t.Skip("set CAPTN_PROBE=1 to probe rust-analyzer")
	}

	ctx := t.Context()

	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	opts := lsp.NewStartOptions(cwd, "captn-probe", "0.1.0", languages.Rust.NewLSPServer)
	opts.InitializationOptions = languages.Rust.GetLSPInitializationOptions(ctx, cwd)

	client, err := lsp.Start(ctx, opts)
	if !assert.NoError(t, err) {
		return
	}

	defer client.Close(ctx)

	rustProbeFile(t, ctx, client, cwd, "./fixtures/rust/multidep/src/main.rs")
	rustProbeFile(t, ctx, client, cwd, "./fixtures/rust/baseproj/src/traits.rs")
	rustProbeFile(t, ctx, client, cwd, "./fixtures/rust/baseproj/src/method.rs")
}
