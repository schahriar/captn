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

func phpCollectTypes(n ast.ASTNode, out *[]*ast.ASTTypeExpression) {
	if texpr, ok := n.(*ast.ASTTypeExpression); ok {
		*out = append(*out, texpr)
	}

	for _, child := range n.Children() {
		phpCollectTypes(child, out)
	}
}

func phpProbeFile(t *testing.T, ctx context.Context, client *lsp.Client, cwd string, path string) {
	t.Helper()

	pf, err := cog.ParseFile(ctx, cwd, path)
	if !assert.NoError(t, err) {
		return
	}

	doc := lsp.NewTextDocumentItem(
		lsp.FileURI(pf.Source.Path),
		languages.PHP.GetLanguageID(),
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

		// The workspace indexes in the background; definitions answer empty
		// until it lands, so retry rather than report a false negative
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
	phpCollectTypes(pf.Module, &types)

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

// TestPHPLSPProbe reports the raw locations intelephense returns, before
// captn interprets them. What comes back here decides how ClassifyImportType
// and NormalizeDefinitionRange must be written.
func TestPHPLSPProbe(t *testing.T) {
	if os.Getenv("CAPTN_PROBE") == "" {
		t.Skip("set CAPTN_PROBE=1 to probe intelephense")
	}

	ctx := t.Context()

	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	scratch := cwd + "/fixtures/php/baseproj/probe_scratch.php"

	assert.NoError(t, os.WriteFile(scratch, []byte(
		"<?php\n"+
			"require_once 'method.php';\n"+
			"$w = new Widget(\"a\");\n"+
			"echo $w->describe(\"p\");\n",
	), 0o644))

	defer os.Remove(scratch)

	opts := lsp.NewStartOptions(cwd, "captn-probe", "0.1.0", languages.PHP.NewLSPServer)
	opts.InitializationOptions = languages.PHP.GetLSPInitializationOptions(ctx, cwd)

	client, err := lsp.Start(ctx, opts)
	if !assert.NoError(t, err) {
		return
	}

	defer client.Close(ctx)

	phpProbeFile(t, ctx, client, cwd, "./fixtures/php/multidep/main.php")
	phpProbeFile(t, ctx, client, cwd, "./fixtures/php/baseproj/types.php")
	phpProbeFile(t, ctx, client, cwd, "./fixtures/php/baseproj/probe_scratch.php")
}
