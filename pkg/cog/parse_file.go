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
	lookupTable map[common.HashType]ast.ASTNode
	intervals   *interval.SearchTree[common.HashType, common.FilePosition]

	Source   *common.Source
	Module   *ast.ASTModule
	Language languages.LanguageSupport
}

func NewCOGFile(src *common.Source, module *ast.ASTModule, lang languages.LanguageSupport) *COGFile {
	return &COGFile{
		Source:   src,
		Module:   module,
		Language: lang,
	}
}

var ErrSnippetNotFound = errors.New("snippet not found")

func offsetToLineColumn(buf []byte, offset int) (int, int) {
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

	return row, col
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

	startLine, startCol := offsetToLineColumn(f.Source.Buffer, startOffset)
	endLine, endCol := offsetToLineColumn(f.Source.Buffer, endOffset)

	return common.NewFileRangeAutoBytePosition(f.Source, startLine, startCol, endLine, endCol)
}

func (f COGFile) GetHash() common.HashType {
	return common.PrimaryHash(f.Source.RelativePath())
}

func (f COGFile) GetFilePath() string {
	return f.Source.Path
}

func (f COGFile) GetFileRange() *common.FileRange {
	return common.NewFileRange(
		f.Source,
		common.FirstPositionOfSource(f.Source),
		common.LastPositionOfSource(f.Source),
	)
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

	lang, ok := languages.ForExtension(ext)

	// An unsupported extension still parses: a whole-file module observations
	// can anchor to, with no grammar and no LSP behind it
	if !ok {
		task.End()

		root, err := languages.Plaintext.Parse(ctx, src, nil)

		if err != nil {
			return nil, fmt.Errorf("failed to parse %v file: %w", ext, err)
		}

		pf := NewCOGFile(src, root, languages.Plaintext)

		pf.IndexNodes()

		return pf, nil
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
		return nil, fmt.Errorf("failed to parse %v file: %w", ext, err)
	}

	pf := NewCOGFile(src, root, lang)

	pf.IndexNodes()

	task.End()

	return pf, nil
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
