package cog

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime/trace"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/languages"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

type ParsedFile struct {
	Source   *common.Source
	Module   *ast.ASTModule
	Language languages.LanguageSupport
}

func ParseSource(ctx context.Context, src *common.Source) (ParsedFile, error) {
	ctx, task := trace.NewTask(ctx, "treeSitterParse")

	ext := filepath.Ext(src.Path)

	var lang languages.LanguageSupport

	if ext == ".go" {
		lang = languages.Golang
	} else {
		return ParsedFile{}, errors.New("Unsupported file type") // TODO: Use knownerrors
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
		return ParsedFile{}, fmt.Errorf("Failed to parse Go code: %w", err)
	}

	task.End()

	return ParsedFile{
		Source:   src,
		Module:   root,
		Language: lang,
	}, nil
}

func ParseFile(ctx context.Context, workspace string, file string) (ParsedFile, error) {
	ctx, task := trace.NewTask(ctx, "loadFile")

	path := filepath.Join(workspace, file)

	src, err := common.NewSourceFromFile(ctx, workspace, path)

	if err != nil {
		panic(fmt.Errorf("Failed to read main file", err))
	}

	task.End()

	return ParseSource(ctx, src)
}
