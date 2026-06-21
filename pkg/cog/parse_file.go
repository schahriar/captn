package cog

import (
	"bytes"
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

var ErrSnippetNotFound = errors.New("snippet not found")

func offsetToPosition(buf []byte, offset int) common.FilePosition {
	if offset < 0 {
		offset = 0
	}
	if offset > len(buf) {
		offset = len(buf)
	}

	row := 0
	col := 0

	for i := 0; i < offset; i++ {
		if buf[i] == '\n' {
			row++
			col = 0
		} else {
			col++
		}
	}

	return common.FilePosition{
		Line:   row,
		Column: col,
	}
}

func (f *COGFile) FindSnippetRange(snippet []byte) (*common.FileRange, error) {
	if len(snippet) == 0 {
		return common.NewFileRangeAutoBytePosition(f.Source, 0, 0, 0, 0)
	}

	startOffset := bytes.Index(f.Source.Buffer, snippet)
	if startOffset < 0 {
		return nil, ErrSnippetNotFound
	}

	endOffset := startOffset + len(snippet)

	start := offsetToPosition(f.Source.Buffer, startOffset)
	end := offsetToPosition(f.Source.Buffer, endOffset)

	return common.NewFileRangeAutoBytePosition(f.Source, start.Line, start.Column, end.Line, end.Column)
}

func (f COGFile) GetHash() uint32 {
	return common.PrimaryHash(f.Source.Path)
}

func (f COGFile) GetFilePath() string {
	return f.Source.Path
}

func (f COGFile) GetStringSource() string {
	return string(f.Source.Buffer)
}

func (f COGFile) GetLanguage() string {
	return f.Source.GetLanguage()
}

func ParseSource(ctx context.Context, src *common.Source) (*COGFile, error) {
	ctx, task := trace.NewTask(ctx, "treeSitterParse")
	ext := filepath.Ext(src.Path)

	var lang languages.LanguageSupport

	if ext == ".go" {
		lang = languages.Golang
	} else {
		return &COGFile{}, errors.New("unsupported file type") // TODO: Use knownerrors
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
		return nil, fmt.Errorf("failed to parse Go code: %w", err)
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
		return nil, fmt.Errorf("failed to read main file %w", err)
	}

	task.End()

	return ParseSource(ctx, src)
}
