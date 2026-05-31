package tests_test

import (
	"encoding/json"
	"testing"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/knownerr"
	"github.com/stretchr/testify/assert"
)

type mockParserNode struct {
	source   *common.Source
	position common.FileRange
}

func (m *mockParserNode) GetSource() *common.Source     { return m.source }
func (m *mockParserNode) GetPosition() common.FileRange { return m.position }

func newTestContainer(src *common.Source, startByte, endByte int) *ast.ASTNodeContainer {
	fpStart := common.NewFilePosition(src, 0, 0, startByte)
	fpEnd := common.NewFilePosition(src, 0, endByte-startByte, endByte)
	node := &mockParserNode{
		source:   src,
		position: common.NewFileRange(src, fpStart, fpEnd),
	}
	return ast.NewASTNodeContainer(node)
}

func catchPanic(fn func()) (recovered interface{}) {
	defer func() { recovered = recover() }()
	fn()
	return
}

func TestASTNodeContainerGetRawSource(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("hello world")}
	cont := newTestContainer(src, 6, 11)
	assert.Equal(t, []byte("world"), cont.GetRawSource())
}

func TestASTNodeContainerGetPosition(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("hello")}
	cont := newTestContainer(src, 0, 5)
	pos := cont.GetPosition()
	assert.Equal(t, 0, pos.Start.BytePosition)
	assert.Equal(t, 5, pos.End.BytePosition)
}

func TestASTNodeContainerDebugPosition(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("hello")}
	cont := newTestContainer(src, 0, 5)
	dbg := cont.DebugPosition()
	assert.Equal(t, "test.go:0:0-0:5", dbg.Position)
	assert.NotEmpty(t, dbg.SourceHash)
}

func TestASTNodeContainerMarshalJSON(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("hello")}
	cont := newTestContainer(src, 0, 5)
	b, err := cont.MarshalJSON()
	assert.NoError(t, err)
	var result map[string]string
	assert.NoError(t, json.Unmarshal(b, &result))
	assert.Equal(t, "test.go:0:0-0:5", result["Position"])
	assert.NotEmpty(t, result["SourceHash"])
}

func TestASTNodeContainerMarshalYAML(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("hello")}
	cont := newTestContainer(src, 0, 5)
	b, err := cont.MarshalYAML()
	assert.NoError(t, err)
	assert.Contains(t, string(b), "test.go:0:0-0:5")
}

func TestASTSymbolFields(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("foo")}
	cont := newTestContainer(src, 0, 3)
	sym := ast.NewASTSymbol(cont, "foo")
	assert.Equal(t, "foo", sym.Name)
	assert.Len(t, sym.Children(), 0)
	assert.Equal(t, cont, sym.GetContainer())
}

func TestASTSymbolAppendChildPanics(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("x")}
	cont := newTestContainer(src, 0, 1)
	sym := ast.NewASTSymbol(cont, "x")
	assert.Panics(t, func() { sym.AppendChild(sym) })
}

func TestASTBlockEmpty(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("{}")}
	cont := newTestContainer(src, 0, 2)
	block := ast.NewASTBlock(cont)
	assert.Len(t, block.Children(), 0)
}

func TestASTBlockAppendChild(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("{x}")}
	cont := newTestContainer(src, 0, 3)
	block := ast.NewASTBlock(cont)
	sym := ast.NewASTSymbol(cont, "x")
	block.AppendChild(sym)
	assert.Len(t, block.Children(), 1)
	assert.Equal(t, sym, block.Children()[0])
}

func TestASTModuleFields(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("module")}
	cont := newTestContainer(src, 0, 6)
	mod := ast.NewASTModule(cont, "mymod")
	assert.Equal(t, "mymod", mod.Name)
	assert.NotNil(t, mod.Block)
	assert.Equal(t, cont, mod.GetContainer())
}

func TestASTModuleChildren(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("module")}
	cont := newTestContainer(src, 0, 6)
	mod := ast.NewASTModule(cont, "mymod")
	children := mod.Children()
	assert.Len(t, children, 1)
	assert.Equal(t, mod.Block, children[0])
}

func TestASTModuleAppendChild(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("module x")}
	cont := newTestContainer(src, 0, 8)
	mod := ast.NewASTModule(cont, "mymod")
	sym := ast.NewASTSymbol(cont, "x")
	mod.AppendChild(sym)
	assert.Len(t, mod.Block.Children(), 1)
	assert.Equal(t, sym, mod.Block.Children()[0])
}

func TestASTImportStatementFields(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("import foo")}
	cont := newTestContainer(src, 0, 10)
	imp := ast.NewASTImportStatement(cont)
	assert.Nil(t, imp.Namespace)
	assert.Nil(t, imp.Reference)
	assert.Len(t, imp.Children(), 0)
}

func TestASTImportStatementAppendChildPanics(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("import foo")}
	cont := newTestContainer(src, 0, 10)
	imp := ast.NewASTImportStatement(cont)
	sym := ast.NewASTSymbol(cont, "foo")
	assert.Panics(t, func() { imp.AppendChild(sym) })
}

func TestASTFuncArgumentFields(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("x int")}
	cont := newTestContainer(src, 0, 5)
	id := ast.NewASTSymbol(cont, "x")
	tp := ast.NewASTSymbol(cont, "int")
	arg := ast.NewASTFuncArgument(cont, id, tp)
	assert.Equal(t, id, arg.Identifier)
	assert.Equal(t, tp, arg.Type)
	assert.Len(t, arg.Children(), 0)
}

func TestASTFuncArgumentAppendChildPanics(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("x int")}
	cont := newTestContainer(src, 0, 5)
	arg := ast.NewASTFuncArgument(cont, nil, nil)
	sym := ast.NewASTSymbol(cont, "x")
	assert.Panics(t, func() { arg.AppendChild(sym) })
}

func TestASTFuncExpressionDefault(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("func(){}")}
	cont := newTestContainer(src, 0, 8)
	fn := ast.NewASTFuncExpression(cont)
	assert.Nil(t, fn.Name)
	assert.Len(t, fn.Arguments, 0)
	assert.Nil(t, fn.ReturnType)
	assert.NotNil(t, fn.Block)
}

func TestASTFuncExpressionChildren(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("func(x int){}")}
	cont := newTestContainer(src, 0, 13)
	fn := ast.NewASTFuncExpression(cont)
	arg := ast.NewASTFuncArgument(cont, nil, nil)
	fn.Arguments = append(fn.Arguments, arg)
	children := fn.Children()
	assert.Len(t, children, 2)
	assert.Equal(t, arg, children[0])
	assert.Equal(t, fn.Block, children[1])
}

func TestASTFuncExpressionAppendChild(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("func(){x}")}
	cont := newTestContainer(src, 0, 9)
	fn := ast.NewASTFuncExpression(cont)
	sym := ast.NewASTSymbol(cont, "x")
	fn.AppendChild(sym)
	assert.Len(t, fn.Block.Children(), 1)
	assert.Equal(t, sym, fn.Block.Children()[0])
}

func TestASTDeclarationEmpty(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("x := 1")}
	cont := newTestContainer(src, 0, 6)
	decl := ast.NewASTDeclaration(cont)
	assert.Len(t, decl.Names, 0)
	assert.Len(t, decl.Children(), 0)
}

func TestASTDeclarationChildren(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("x := 1")}
	cont := newTestContainer(src, 0, 6)
	decl := ast.NewASTDeclaration(cont)
	sym := ast.NewASTSymbol(cont, "x")
	decl.Names = append(decl.Names, sym)
	ret := ast.NewReturnStatement(cont)
	decl.AppendChild(ret)
	children := decl.Children()
	assert.Len(t, children, 2)
	assert.Equal(t, sym, children[0])
	assert.Equal(t, ret, children[1])
}

func TestASTDeclarationAppendChild(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("x := 1")}
	cont := newTestContainer(src, 0, 6)
	decl := ast.NewASTDeclaration(cont)
	sym := ast.NewASTSymbol(cont, "x")
	decl.AppendChild(sym)
	assert.Len(t, decl.Children(), 1)
	assert.Equal(t, sym, decl.Children()[0])
}

func TestASTReturnStatementEmpty(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("return")}
	cont := newTestContainer(src, 0, 6)
	ret := ast.NewReturnStatement(cont)
	assert.Len(t, ret.Children(), 0)
}

func TestASTReturnStatementAppendChild(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("return x")}
	cont := newTestContainer(src, 0, 8)
	ret := ast.NewReturnStatement(cont)
	sym := ast.NewASTSymbol(cont, "x")
	ret.AppendChild(sym)
	children := ret.Children()
	assert.Len(t, children, 1)
	assert.Equal(t, sym, children[0])
}

func TestASTCallExpressionDefault(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("foo()")}
	cont := newTestContainer(src, 0, 5)
	call := ast.NewASTCallExpression(cont)
	assert.Nil(t, call.Namespace)
	assert.Nil(t, call.Symbol)
	assert.Len(t, call.Arguments, 0)
}

func TestASTCallExpressionChildren(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("foo(x)")}
	cont := newTestContainer(src, 0, 6)
	call := ast.NewASTCallExpression(cont)
	arg := ast.NewASTFuncArgument(cont, nil, nil)
	call.Arguments = append(call.Arguments, arg)
	sym := ast.NewASTSymbol(cont, "x")
	call.AppendChild(sym)
	children := call.Children()
	assert.Len(t, children, 2)
	assert.Equal(t, arg, children[0])
	assert.Equal(t, sym, children[1])
}

func TestASTCallExpressionAppendChild(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("foo()")}
	cont := newTestContainer(src, 0, 5)
	call := ast.NewASTCallExpression(cont)
	sym := ast.NewASTSymbol(cont, "x")
	call.AppendChild(sym)
	assert.Len(t, call.Virtual, 1)
	assert.Equal(t, sym, call.Virtual[0])
}

func TestASTPanicBoundaryNoError(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("x")}
	cont := newTestContainer(src, 0, 1)
	sym := ast.NewASTSymbol(cont, "x")
	result := ast.ASTPanicBoundary(sym, func() interface{} {
		return 42
	})
	assert.Equal(t, 42, result)
}

func TestASTPanicBoundaryRecoverable(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("x")}
	cont := newTestContainer(src, 0, 1)
	sym := ast.NewASTSymbol(cont, "x")
	r := catchPanic(func() {
		ast.ASTPanicBoundary(sym, func() interface{} {
			panic(knownerr.UnresolvedType())
		})
	})
	astErr, ok := r.(ast.ASTError)
	assert.True(t, ok)
	assert.Equal(t, sym, astErr.Node)
}

func TestASTPanicBoundaryNested(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("hello world")}
	outer := ast.NewASTSymbol(newTestContainer(src, 0, 5), "outer")
	inner := ast.NewASTSymbol(newTestContainer(src, 6, 11), "inner")
	r := catchPanic(func() {
		ast.ASTPanicBoundary(outer, func() interface{} {
			return ast.ASTPanicBoundary(inner, func() interface{} {
				panic(knownerr.UnresolvedType())
			})
		})
	})
	astErr, ok := r.(ast.ASTError)
	assert.True(t, ok)
	stack := astErr.StackWithMaxDepth(10)
	assert.Len(t, stack.Stack, 2)
	assert.Equal(t, outer, stack.Stack[0])
	assert.Equal(t, inner, stack.Stack[1])
	assert.NotNil(t, stack.Error)
}

func TestASTPanicBoundaryNonRecoverable(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("x")}
	cont := newTestContainer(src, 0, 1)
	sym := ast.NewASTSymbol(cont, "x")
	sentinel := "plain string panic"
	r := catchPanic(func() {
		ast.ASTPanicBoundary(sym, func() interface{} {
			panic(sentinel)
		})
	})
	_, ok := r.(ast.ASTError)
	assert.False(t, ok)
	assert.Equal(t, sentinel, r)
}

func TestASTErrorStackMaxDepth(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("a b c")}
	a := ast.NewASTSymbol(newTestContainer(src, 0, 1), "a")
	b := ast.NewASTSymbol(newTestContainer(src, 2, 3), "b")
	c := ast.NewASTSymbol(newTestContainer(src, 4, 5), "c")
	r := catchPanic(func() {
		ast.ASTPanicBoundary(a, func() interface{} {
			return ast.ASTPanicBoundary(b, func() interface{} {
				return ast.ASTPanicBoundary(c, func() interface{} {
					panic(knownerr.UnresolvedType())
				})
			})
		})
	})
	astErr, ok := r.(ast.ASTError)
	assert.True(t, ok)
	stack := astErr.StackWithMaxDepth(2)
	assert.Len(t, stack.Stack, 2)
}

func TestImportVisitorCollectsImports(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("import foo\nimport bar\n")}
	cont := newTestContainer(src, 0, len(src.Buffer))
	mod := ast.NewASTModule(cont, "test")
	imp1 := ast.NewASTImportStatement(cont)
	imp2 := ast.NewASTImportStatement(cont)
	mod.AppendChild(imp1)
	mod.AppendChild(imp2)
	vis := &ast.ImportVisitor{}
	mod.Accept(vis)
	assert.Len(t, vis.Imports, 2)
	assert.Equal(t, imp1, vis.Imports[0])
	assert.Equal(t, imp2, vis.Imports[1])
}

func TestInspectingVisitorOnModule(t *testing.T) {
	src := &common.Source{Path: "test.go", Buffer: []byte("func foo() {}")}
	cont := newTestContainer(src, 0, len(src.Buffer))
	mod := ast.NewASTModule(cont, "test")
	vis := &ast.InspectingVisitor{}
	result := mod.Accept(vis)
	assert.NotNil(t, result)
}
