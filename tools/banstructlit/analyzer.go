package main

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

const modulePath = "github.com/schahriar/captn"
const ignoreDirective = "banstructlit:ignore"

var Analyzer = &analysis.Analyzer{
	Name:     "banstructlit",
	Doc:      "bans direct struct instantiation; constructors must be named New<Struct>",
	Requires: []*analysis.Analyzer{inspect.Analyzer},
	Run:      run,
}

func run(pass *analysis.Pass) (interface{}, error) {
	insp := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

	filter := []ast.Node{
		(*ast.CompositeLit)(nil),
		(*ast.CallExpr)(nil),
	}

	insp.WithStack(filter, func(n ast.Node, push bool, stack []ast.Node) bool {
		if !push {
			return true
		}
		switch x := n.(type) {
		case *ast.CompositeLit:
			checkComposite(pass, x, stack)
		case *ast.CallExpr:
			checkNewCall(pass, x, stack)
		}
		return true
	})

	return nil, nil
}

func checkComposite(pass *analysis.Pass, lit *ast.CompositeLit, stack []ast.Node) {
	if isExcludedFile(pass, lit.Pos()) {
		return
	}
	if ignoredByDirective(pass, lit.Pos(), stack) {
		return
	}
	named := namedStruct(pass.TypesInfo.TypeOf(lit))
	if named == nil || !inThisModule(named) {
		return
	}
	if allowedByConstructor(pass, named, stack) {
		return
	}
	pass.Reportf(lit.Pos(), "direct instantiation of %s is banned; use %s constructor", named.Obj().Name(), suggestedConstructor(named))
}

func checkNewCall(pass *analysis.Pass, call *ast.CallExpr, stack []ast.Node) {
	if isExcludedFile(pass, call.Pos()) {
		return
	}
	if ignoredByDirective(pass, call.Pos(), stack) {
		return
	}
	id, ok := call.Fun.(*ast.Ident)
	if !ok || id.Name != "new" || len(call.Args) != 1 {
		return
	}
	if _, isBuiltin := pass.TypesInfo.Uses[id].(*types.Builtin); !isBuiltin {
		return
	}
	named := namedStruct(pass.TypesInfo.TypeOf(call.Args[0]))
	if named == nil || !inThisModule(named) {
		return
	}
	if allowedByConstructor(pass, named, stack) {
		return
	}
	pass.Reportf(call.Pos(), "new(%s) is banned; use %s constructor", named.Obj().Name(), suggestedConstructor(named))
}

func suggestedConstructor(named *types.Named) string {
	name := named.Obj().Name()
	if named.Obj().Exported() {
		return "New" + name
	}
	return "new" + strings.ToUpper(name[:1]) + name[1:]
}

func namedStruct(t types.Type) *types.Named {
	if t == nil {
		return nil
	}
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return nil
	}
	if _, ok := named.Underlying().(*types.Struct); !ok {
		return nil
	}
	return named
}

func isExcludedFile(pass *analysis.Pass, pos token.Pos) bool {
	file := pass.Fset.Position(pos).Filename
	return strings.Contains(file, "/tests/")
}

func hasIgnoreCommentAbove(pass *analysis.Pass, pos token.Pos) bool {
	for _, file := range pass.Files {
		if !fileContainsPos(file, pos) {
			continue
		}
		return ignoreDirectiveAbove(file, pass.Fset, pos)
	}
	return false
}

func ignoredByDirective(pass *analysis.Pass, pos token.Pos, stack []ast.Node) bool {
	if hasIgnoreCommentAbove(pass, pos) {
		return true
	}
	for i := len(stack) - 1; i >= 0; i-- {
		stmt, ok := stack[i].(ast.Stmt)
		if !ok {
			continue
		}
		return hasIgnoreCommentAbove(pass, stmt.Pos())
	}
	return false
}

func fileContainsPos(file *ast.File, pos token.Pos) bool {
	return file.Pos() <= pos && pos <= file.End()
}

func ignoreDirectiveAbove(file *ast.File, fset *token.FileSet, pos token.Pos) bool {
	nodePos := fset.Position(pos)
	for _, group := range file.Comments {
		if fset.Position(group.End()).Line != nodePos.Line-1 {
			continue
		}
		for _, comment := range group.List {
			if strings.Contains(commentText(comment.Text), ignoreDirective) {
				return true
			}
		}
	}
	return false
}

func commentText(text string) string {
	if strings.HasPrefix(text, "//") {
		return strings.TrimSpace(strings.TrimPrefix(text, "//"))
	}
	text = strings.TrimPrefix(text, "/*")
	text = strings.TrimSuffix(text, "*/")
	return strings.TrimSpace(text)
}

func inThisModule(named *types.Named) bool {
	pkg := named.Obj().Pkg()
	if pkg == nil {
		return false
	}
	path := pkg.Path()
	return path == modulePath || strings.HasPrefix(path, modulePath+"/")
}

func allowedByConstructor(pass *analysis.Pass, named *types.Named, stack []ast.Node) bool {
	if named.Obj().Pkg() != pass.Pkg {
		return false
	}
	name := named.Obj().Name()
	exportedForm := "New" + name
	unexportedForm := "new" + strings.ToUpper(name[:1]) + name[1:]
	for i := len(stack) - 1; i >= 0; i-- {
		fn, ok := stack[i].(*ast.FuncDecl)
		if !ok {
			continue
		}
		return fn.Name.Name == exportedForm || fn.Name.Name == unexportedForm
	}
	return false
}
