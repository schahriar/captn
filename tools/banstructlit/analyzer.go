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
