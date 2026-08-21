package tests_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/languages"
	"github.com/schahriar/captn/pkg/lsp"
	"github.com/stretchr/testify/assert"
)

func probeFileWith(t *testing.T, lang languages.LanguageSupport, path string) {
	t.Helper()

	ctx := t.Context()

	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	pf, err := cog.ParseFile(ctx, cwd, path)
	if !assert.NoError(t, err) {
		return
	}

	opts := lsp.NewStartOptions(cwd, "captn-probe", "0.1.0", lang.NewLSPServer)
	opts.InitializationOptions = lang.GetLSPInitializationOptions(ctx, cwd)

	client, err := lsp.Start(ctx, opts)
	if !assert.NoError(t, err) {
		return
	}

	defer client.Close(ctx)

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

		refs, err := client.ImportDefinition(ctx, doc, imp.GetPosition())

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

	fmt.Println()
}

// TestCSSLSPProbe reports the raw locations vscode-css-language-server
// returns, before captn interprets them. What comes back here decides how
// ClassifyImportType and NormalizeDefinitionRange must be written.
func TestCSSLSPProbe(t *testing.T) {
	if os.Getenv("CAPTN_PROBE") == "" {
		t.Skip("set CAPTN_PROBE=1 to probe vscode-css-language-server")
	}

	probeFileWith(t, languages.CSS, "./fixtures/css/baseproj/styles.css")
	probeFileWith(t, languages.CSS, "./fixtures/css/multidep/main.css")
}

// TestSCSSLSPProbe asks the css server for a definition at a $variable use
// with languageId "scss": preprocessor syntax is invisible to the degraded
// parse, so this is the only place the dialect claim meets the real server.
func TestSCSSLSPProbe(t *testing.T) {
	if os.Getenv("CAPTN_PROBE") == "" {
		t.Skip("set CAPTN_PROBE=1 to probe the scss dialect")
	}

	ctx := t.Context()

	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	pf, err := cog.ParseFile(ctx, cwd, "./fixtures/css/baseproj/degraded.scss")
	if !assert.NoError(t, err) {
		return
	}

	opts := lsp.NewStartOptions(cwd, "captn-probe", "0.1.0", languages.SCSS.NewLSPServer)

	client, err := lsp.Start(ctx, opts)
	if !assert.NoError(t, err) {
		return
	}

	defer client.Close(ctx)

	doc := lsp.NewTextDocumentItem(
		lsp.FileURI(pf.Source.Path),
		languages.SCSS.GetLanguageID(),
		1,
		string(pf.Source.Buffer),
	)

	// The lone `$accent;` spelling is the use site; the declaration spells
	// `$accent:`
	rng, err := pf.FindSnippetRange([]byte("$accent;"))
	if !assert.NoError(t, err) {
		return
	}

	refs, err := client.Definition(ctx, lsp.DefinitionRequest{TextDocument: doc, Range: rng})

	if err != nil {
		fmt.Printf("  scss $accent use ERROR %v\n", err)
		return
	}

	if len(refs) == 0 {
		fmt.Printf("  scss $accent use (no definition)\n")
		return
	}

	for _, ref := range refs {
		fmt.Printf("  scss $accent use -> %v %d:%d-%d:%d\n", ref.URI, ref.Range.Start.Line, ref.Range.Start.Character, ref.Range.End.Line, ref.Range.End.Character)
	}
}
