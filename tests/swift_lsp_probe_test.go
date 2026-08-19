package tests_test

import (
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

func collectCalls(n ast.ASTNode, out *[]*ast.ASTCallExpression) {
	if call, ok := n.(*ast.ASTCallExpression); ok {
		*out = append(*out, call)
	}

	for _, child := range n.Children() {
		collectCalls(child, out)
	}
}

// TestSwiftLSPProbe reports the raw locations sourcekit-lsp returns, before
// captn interprets them. Swift resolves imports to modules rather than files and
// keeps its standard library in generated interfaces, so what comes back here
// decides how ClassifyImportType and the extension dispatch must be written.
func TestSwiftLSPProbe(t *testing.T) {
	if os.Getenv("CAPTN_PROBE") == "" {
		t.Skip("set CAPTN_PROBE=1 to probe sourcekit-lsp")
	}

	ctx := t.Context()

	cwd, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	pf, err := cog.ParseFile(ctx, cwd, "./fixtures/swift/multidep/Sources/App/main.swift")
	if !assert.NoError(t, err) {
		return
	}

	client, err := lsp.Start(ctx, lsp.NewStartOptions(cwd, "captn-probe", "0.1.0", languages.Swift.NewLSPServer))
	if !assert.NoError(t, err) {
		return
	}

	defer client.Close(ctx)

	doc := lsp.NewTextDocumentItem(
		lsp.FileURI(pf.Source.Path),
		languages.Swift.GetLanguageID(),
		1,
		string(pf.Source.Buffer),
	)

	// Background indexing builds the package first; definitions are empty until
	// it lands, so the probe waits rather than reporting a false negative
	var calls []*ast.ASTCallExpression
	collectCalls(pf.Module, &calls)

	var probe *ast.ASTCallExpression

	for _, call := range calls {
		if call.Symbol != nil && call.Symbol.Name == "getExampleText" {
			probe = call
		}
	}

	if !assert.NotNil(t, probe, "expected the cross-target call in the fixture") {
		return
	}

	for attempt := 1; attempt <= 12; attempt++ {
		refs, err := client.Definition(ctx, lsp.DefinitionRequest{TextDocument: doc, Range: probe.Symbol.GetPosition()})

		if err == nil && len(refs) > 0 {
			fmt.Printf("\nindexed after %d attempt(s)\n", attempt)
			break
		}

		if attempt == 12 {
			fmt.Printf("\ncross-target definition never resolved (err=%v)\n", err)
		}

		time.Sleep(2 * time.Second)
	}

	fmt.Printf("\n=== imports ===\n")

	impVis := ast.NewImportVisitor()
	pf.Module.Accept(impVis)

	for _, imp := range impVis.Imports {
		refs, err := client.ImportDefinition(ctx, doc, imp.GetPosition())

		name := "?"
		if imp.Reference != nil {
			name = imp.Reference.Name
		}

		if err != nil {
			fmt.Printf("  import %-12s ERROR %v\n", name, err)
			continue
		}

		if len(refs) == 0 {
			fmt.Printf("  import %-12s (no definition)\n", name)
			continue
		}

		for _, ref := range refs {
			fmt.Printf("  import %-12s -> %v %d:%d-%d:%d\n", name, filepath.Base(ref.URI), ref.Range.Start.Line, ref.Range.Start.Character, ref.Range.End.Line, ref.Range.End.Character)
		}
	}

	fmt.Printf("\n=== call expressions ===\n")

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
			fmt.Printf("  call %-16s -> %v %d:%d-%d:%d\n", call.Symbol.Name, filepath.Base(ref.URI), ref.Range.Start.Line, ref.Range.Start.Character, ref.Range.End.Line, ref.Range.End.Character)
		}
	}

	fmt.Println()
}
