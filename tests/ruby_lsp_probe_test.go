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

func rbCollectTypes(n ast.ASTNode, out *[]*ast.ASTTypeExpression) {
	if texpr, ok := n.(*ast.ASTTypeExpression); ok {
		*out = append(*out, texpr)
	}

	for _, child := range n.Children() {
		rbCollectTypes(child, out)
	}
}

func rbProbeFile(t *testing.T, ctx context.Context, client *lsp.Client, cwd string, path string) {
	t.Helper()

	pf, err := cog.ParseFile(ctx, cwd, path)
	if !assert.NoError(t, err) {
		return
	}

	doc := lsp.NewTextDocumentItem(
		lsp.FileURI(pf.Source.Path),
		languages.Ruby.GetLanguageID(),
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
	rbCollectTypes(pf.Module, &types)

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

// TestRubyLSPProbe reports the raw locations ruby-lsp returns, before captn
// interprets them. What comes back here decides how ClassifyImportType and
// NormalizeDefinitionRange must be written.
func TestRubyLSPProbe(t *testing.T) {
	if os.Getenv("CAPTN_PROBE") == "" {
		t.Skip("set CAPTN_PROBE=1 to probe ruby-lsp")
	}

	ctx := t.Context()

	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	scratch := cwd + "/fixtures/ruby/baseproj/probe_scratch.rb"

	assert.NoError(t, os.WriteFile(scratch, []byte(
		"require_relative \"method\"\n"+
			"w = Widget.new(\"a\")\n"+
			"w.label\n"+
			"w.describe(\"p\")\n",
	), 0o644))

	defer os.Remove(scratch)

	// ruby-lsp's indexer hard-excludes **/fixtures/**, so each fixture
	// project must be its own workspace root for its definitions to resolve
	for workspace, files := range map[string][]string{
		cwd + "/fixtures/ruby/multidep": {"main.rb"},
		cwd + "/fixtures/ruby/baseproj": {"types.rb", "probe_scratch.rb"},
	} {
		opts := lsp.NewStartOptions(workspace, "captn-probe", "0.1.0", languages.Ruby.NewLSPServer)
		opts.InitializationOptions = languages.Ruby.GetLSPInitializationOptions(ctx, workspace)

		client, err := lsp.Start(ctx, opts)
		if !assert.NoError(t, err) {
			return
		}

		// ruby-lsp indexes the workspace in the background after initialize
		// and answers empty until it lands
		time.Sleep(10 * time.Second)

		for _, file := range files {
			rbProbeFile(t, ctx, client, workspace, file)
		}

		client.Close(ctx)
	}
}
