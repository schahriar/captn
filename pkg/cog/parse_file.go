package cog

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime/trace"

	"github.com/rdleal/intervalst/interval"
	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/languages"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

type COGFile struct {
	isIndexed   bool
	lookupTable map[uint32]ast.ASTNode
	intervals   *interval.SearchTree[uint32, common.FilePosition]

	Source   *common.Source
	Module   *ast.ASTModule
	Language languages.LanguageSupport
}

func (f COGFile) GetHash() string {
	return f.Source.Path
}

func ParseSource(ctx context.Context, src *common.Source) (*COGFile, error) {
	ctx, task := trace.NewTask(ctx, "treeSitterParse")
	ext := filepath.Ext(src.Path)

	var lang languages.LanguageSupport

	if ext == ".go" {
		lang = languages.Golang
	} else {
		return &COGFile{}, errors.New("Unsupported file type") // TODO: Use knownerrors
	}

	tsp := tree_sitter.NewParser()
	defer tsp.Close()
	tsp.SetLanguage(lang.GetTreeSitterLanguage())

	tree := tsp.Parse(src.Buffer, nil)
	defer tree.Close()

	task.End()

	ctx, task = trace.NewTask(ctx, "languageParse")

	root, err := lang.Parse(ctx, src, tree)

	if err != nil {
		return nil, fmt.Errorf("Failed to parse Go code: %w", err)
	}

	pf := COGFile{
		Source:   src,
		Module:   root,
		Language: lang,
	}

	pf.IndexNodes()

	task.End()

	return &pf, nil
}

func ParseFile(ctx context.Context, workspace string, file string) (*COGFile, error) {
	ctx, task := trace.NewTask(ctx, "loadFile")

	path := filepath.Join(workspace, file)

	src, err := common.NewSourceFromFile(ctx, workspace, path)

	if err != nil {
		return nil, fmt.Errorf("Failed to read main file %w", err)
	}

	task.End()

	return ParseSource(ctx, src)
}
