package tests_test

import (
	"os"
	"testing"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/cog"
	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/languages"
	"github.com/stretchr/testify/assert"
)

func tsDeclarationNamed(t *testing.T, pf *cog.COGFile, name string) *ast.ASTDeclaration {
	t.Helper()

	var found *ast.ASTDeclaration

	eachNode(pf.Module, func(node ast.ASTNode) {
		decl, ok := node.(*ast.ASTDeclaration)
		if !ok || found != nil {
			return
		}

		for _, sym := range decl.Names {
			if sym.Name == name {
				found = decl
			}
		}
	})

	if found == nil {
		assert.Failf(t, "declaration not found", "expected a declaration named %q", name)
	}

	return found
}

// A composite with no name of its own folds flat: the first named type is the
// head and the rest are its arguments, one level deep. Only a generic head
// owns its arguments, and predefined types vanish from every fold.
func TestTypescriptTypeExpressionFoldsFlat(t *testing.T) {
	pf := parseInline(t, "fold.ts", "function f(a: Result<Widget, Fault>, m: Record<string, Widget>, h: (a: Widget) => Report, u: Widget | Report, arr: Widget[]) {}\n")
	if pf == nil {
		return
	}

	fn := firstFunction(t, pf)
	if !assert.Len(t, fn.Arguments, 5) {
		return
	}

	if assertTypeSource(t, fn.Arguments[0].Type, "Result"); fn.Arguments[0].Type != nil {
		assert.Equal(t, []string{"Widget", "Fault"}, typeNames(fn.Arguments[0].Type.Arguments))
	}

	if assertTypeSource(t, fn.Arguments[1].Type, "Record"); fn.Arguments[1].Type != nil {
		assert.Equal(t, []string{"Widget"}, typeNames(fn.Arguments[1].Type.Arguments), "predefined types never become arguments")
	}

	if assertTypeSource(t, fn.Arguments[2].Type, "Widget"); fn.Arguments[2].Type != nil {
		assert.Equal(t, []string{"Report"}, typeNames(fn.Arguments[2].Type.Arguments))
	}

	if assertTypeSource(t, fn.Arguments[3].Type, "Widget"); fn.Arguments[3].Type != nil {
		assert.Equal(t, []string{"Report"}, typeNames(fn.Arguments[3].Type.Arguments))
		for _, arg := range fn.Arguments[3].Type.Arguments {
			assert.Empty(t, arg.Arguments, "flat fold never chains siblings")
		}
	}

	assertTypeSource(t, fn.Arguments[4].Type, "Widget")
}

func TestTypescriptPredefinedTypesAreSkipped(t *testing.T) {
	pf := parseInline(t, "predef.ts", "function g(a: number, b: string | undefined): void {}\n")
	if pf == nil {
		return
	}

	fn := firstFunction(t, pf)
	if !assert.Len(t, fn.Arguments, 2) {
		return
	}

	assert.Nil(t, fn.Arguments[0].Type)
	assert.Nil(t, fn.Arguments[1].Type)
	assert.Nil(t, fn.ReturnType)
}

func TestTypescriptNamespacedType(t *testing.T) {
	pf := parseInline(t, "ns.ts", "function f(a: pkg.Thing): pkg.Report { return a; }\n")
	if pf == nil {
		return
	}

	fn := firstFunction(t, pf)
	if !assert.Len(t, fn.Arguments, 1) {
		return
	}

	if assertTypeSource(t, fn.Arguments[0].Type, "Thing"); fn.Arguments[0].Type != nil {
		if assert.NotNil(t, fn.Arguments[0].Type.Namespace) {
			assert.Equal(t, "pkg", fn.Arguments[0].Type.Namespace.Name)
		}
	}

	if assertTypeSource(t, fn.ReturnType, "Report"); fn.ReturnType != nil {
		if assert.NotNil(t, fn.ReturnType.Namespace) {
			assert.Equal(t, "pkg", fn.ReturnType.Namespace.Name)
		}
	}
}

// Type parameters are arguments of the declaring function, ahead of the value
// parameters, and each one leaves exactly one node a definition can resolve onto
func TestTypescriptTypeParametersAreArguments(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/typescript/baseproj/generic.ts")

	fn := namedFunction(t, pf, "mapValues")
	if fn == nil || !assert.Len(t, fn.Arguments, 4) {
		return
	}

	if assert.NotNil(t, fn.Arguments[0].Identifier) {
		assert.Equal(t, "T", fn.Arguments[0].Identifier.Name)
		assert.Nil(t, fn.Arguments[0].Type)
	}

	if assert.NotNil(t, fn.Arguments[1].Identifier) {
		assert.Equal(t, "U", fn.Arguments[1].Identifier.Name)
		assert.Len(t, pf.FindNodesWithinRange(fn.Arguments[1].Identifier.GetPosition()), 1)
	}

	if assert.NotNil(t, fn.Arguments[2].Identifier) {
		assert.Equal(t, "s", fn.Arguments[2].Identifier.Name)
		assertTypeSource(t, fn.Arguments[2].Type, "T")
	}

	if assert.NotNil(t, fn.Arguments[3].Identifier) {
		assert.Equal(t, "f", fn.Arguments[3].Identifier.Name)
		if assertTypeSource(t, fn.Arguments[3].Type, "T"); fn.Arguments[3].Type != nil {
			assert.Equal(t, []string{"U"}, typeNames(fn.Arguments[3].Type.Arguments))
		}
	}

	assertTypeSource(t, fn.ReturnType, "U")

	eachNode(pf.Module, func(node ast.ASTNode) {
		_, ok := node.(*ast.ASTDeclaration)
		assert.False(t, ok, "type parameters must not be declarations")
	})
}

func TestTypescriptDeclaresNamedTypes(t *testing.T) {
	// A resolved type reference needs a node at its definition; without this
	// SearchSnippet fails with "expected 1 node for dependent node"
	pf := parseTestFile(t, "./fixtures/typescript/baseproj/types.ts")

	names := declaredNames(pf.Module)
	assert.Contains(t, names, "Gadget")
	assert.Contains(t, names, "Mode")
	assert.Contains(t, names, "GadgetAlias")
	assert.Contains(t, names, "GadgetPair")
	assert.Contains(t, names, "Labeler")
	assert.Contains(t, names, "Inventory")
	assert.Contains(t, names, "describe")
	assert.Contains(t, names, "firstSerial")

	// A type parameter is an argument of the declaring alias, not a
	// declaration in its block
	assert.NotContains(t, names, "T")
}

func TestTypescriptTypeAlias(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/typescript/baseproj/types.ts")

	// A member-less, non-generic alias is a declaration, not a vertex
	alias := tsDeclarationNamed(t, pf, "GadgetAlias")
	if alias != nil {
		assertTypeSource(t, alias.Type, "Gadget")
		assert.False(t, cog.IsNodeOfInterest(alias))
	}

	// An object-type alias carries members, which get definition-site nodes
	// of their own like an interface's
	pair := namedFunction(t, pf, "GadgetPair")
	if pair != nil && assert.Len(t, pair.Block.Children(), 2) {
		left, ok := pair.Block.Children()[0].(*ast.ASTDeclaration)
		if assert.True(t, ok, "expected a Declaration for the member, got %T", pair.Block.Children()[0]) {
			if assert.Len(t, left.Names, 1) {
				assert.Equal(t, "left", left.Names[0].Name)
			}
			assertTypeSource(t, left.Type, "Gadget")
		}

		right, ok := pair.Block.Children()[1].(*ast.ASTDeclaration)
		if assert.True(t, ok, "expected a Declaration for the member, got %T", pair.Block.Children()[1]) && assert.Len(t, right.Names, 1) {
			assert.Equal(t, "right", right.Names[0].Name)
		}
	}

	// A generic alias stays a function expression so its type parameters
	// keep their definition-site nodes
	labeler := namedFunction(t, pf, "Labeler")
	if labeler != nil && assert.Len(t, labeler.Arguments, 1) {
		if assert.NotNil(t, labeler.Arguments[0].Identifier) {
			assert.Equal(t, "T", labeler.Arguments[0].Identifier.Name)
		}
		assertTypeSource(t, labeler.Arguments[0].Type, "Gadget")
	}
}

// A method member of an object-type alias is where tsserver answers a
// definition of a call through the alias; it needs nodes like an interface's
func TestTypescriptObjectTypeAliasMembers(t *testing.T) {
	pf := parseInline(t, "objalias.ts", "type Codec = { parse(s: Raw): Value; label: Raw };\n")
	if pf == nil {
		return
	}

	codec := namedFunction(t, pf, "Codec")
	if codec == nil || !assert.Len(t, codec.Block.Children(), 2) {
		return
	}

	parse, ok := codec.Block.Children()[0].(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected a FuncExpression for the method member, got %T", codec.Block.Children()[0]) {
		if assert.NotNil(t, parse.Name) {
			assert.Equal(t, "parse", parse.Name.Name)
		}
		assert.Len(t, parse.Arguments, 1)
	}

	rng, err := pf.FindSnippetRange([]byte("parse"))
	if !assert.NoError(t, err) {
		return
	}

	assert.Len(t, pf.FindNodesWithinRange(rng), 1)
}

// A construct signature keeps its return type under grammar field `type`;
// the narrowed reply for `new (): Widget` lands on the return type name
func TestTypescriptConstructSignatureReturnType(t *testing.T) {
	pf := parseInline(t, "ctor.ts", "interface Factory {\n  new (): Widget;\n}\n")
	if pf == nil {
		return
	}

	factory := namedFunction(t, pf, "Factory")
	if factory == nil || !assert.Len(t, factory.Block.Children(), 1) {
		return
	}

	ctor, ok := factory.Block.Children()[0].(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected a FuncExpression for the construct signature, got %T", factory.Block.Children()[0]) {
		assert.Nil(t, ctor.Name)
		assertTypeSource(t, ctor.ReturnType, "Widget")
	}
}

func TestTypescriptGeneratorFunctionExpression(t *testing.T) {
	pf := parseInline(t, "gen.ts", "const g = function* () {\n  yield helper();\n};\n")
	if pf == nil {
		return
	}

	decl := tsDeclarationNamed(t, pf, "g")
	if decl == nil || !assert.Len(t, decl.Virtual, 1) {
		return
	}

	gen, ok := decl.Virtual[0].(*ast.ASTFuncExpression)
	if !assert.True(t, ok, "expected a FuncExpression for the generator, got %T", decl.Virtual[0]) {
		return
	}

	calls := 0
	eachNode(gen, func(node ast.ASTNode) {
		if call, ok := node.(*ast.ASTCallExpression); ok && call.Symbol != nil && call.Symbol.Name == "helper" {
			calls++
		}
	})

	assert.Equal(t, 1, calls, "the yielded call belongs to the generator's block")
}

// A cast's inline object type is a type, not code; its member signatures
// must not leak function vertices into an expression position
func TestTypescriptCastObjectTypeDoesNotLeak(t *testing.T) {
	pf := parseInline(t, "cast.ts", "const cfg = data as { parse(s: string): number };\n")
	if pf == nil {
		return
	}

	eachNode(pf.Module, func(node ast.ASTNode) {
		if fn, ok := node.(*ast.ASTFuncExpression); ok {
			assert.Nil(t, fn.Name, "a cast member must not become a named function vertex")
		}
	})
}

func TestTypescriptMixinExtendsKeepsCall(t *testing.T) {
	pf := parseInline(t, "mixin.ts", "class Widget extends Serializable(Base) {}\n")
	if pf == nil {
		return
	}

	widget := namedFunction(t, pf, "Widget")
	if widget == nil {
		return
	}

	calls := 0
	eachNode(widget, func(node ast.ASTNode) {
		if call, ok := node.(*ast.ASTCallExpression); ok && call.Symbol != nil && call.Symbol.Name == "Serializable" {
			calls++
		}
	})

	assert.Equal(t, 1, calls, "the mixin call is the only path to its definition")
}

func TestTypescriptEnumInitializerKeepsCall(t *testing.T) {
	pf := parseInline(t, "enuminit.ts", "enum Level {\n  Debug = compute(),\n}\n")
	if pf == nil {
		return
	}

	level := namedFunction(t, pf, "Level")
	if level == nil {
		return
	}

	calls := 0
	eachNode(level, func(node ast.ASTNode) {
		if call, ok := node.(*ast.ASTCallExpression); ok && call.Symbol != nil && call.Symbol.Name == "compute" {
			calls++
		}
	})

	assert.Equal(t, 1, calls, "the initializer call belongs to the enum's block")
}

func TestTypescriptRequireCallEmitsImport(t *testing.T) {
	pf := parseInline(t, "reqcall.js", "const util = require(\"./util\");\nfunction run() {\n  const extra = require(\"extra\");\n}\n")
	if pf == nil {
		return
	}

	refs := []string{}
	eachNode(pf.Module, func(node ast.ASTNode) {
		if imp, ok := node.(*ast.ASTImportStatement); ok && imp.Reference != nil {
			refs = append(refs, imp.Reference.Name)
		}
	})

	assert.Equal(t, []string{"./util", "extra"}, refs)
}

func TestTypescriptEnum(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/typescript/baseproj/types.ts")

	mode := namedFunction(t, pf, "Mode")
	if mode == nil {
		return
	}

	members := mode.Block.Children()
	if !assert.Len(t, members, 2) {
		return
	}

	idle, ok := members[0].(*ast.ASTSymbol)
	if assert.True(t, ok, "expected a Symbol for the bare member, got %T", members[0]) {
		assert.Equal(t, "Idle", idle.Name)
	}

	busy, ok := members[1].(*ast.ASTDeclaration)
	if assert.True(t, ok, "expected a Declaration for the assigned member, got %T", members[1]) {
		if assert.Len(t, busy.Names, 1) {
			assert.Equal(t, "Busy", busy.Names[0].Name)
		}
	}

	// A definition resolving onto a member must find exactly one node
	rng, err := pf.FindSnippetRange([]byte("Idle"))
	if !assert.NoError(t, err) {
		return
	}

	assert.Len(t, pf.FindNodesWithinRange(rng), 1)
}

func TestTypescriptInterfaceMembers(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/typescript/baseproj/types.ts")

	inventory := namedFunction(t, pf, "Inventory")
	if inventory == nil {
		return
	}

	members := inventory.Block.Children()
	if !assert.Len(t, members, 3) {
		return
	}

	iterable, ok := members[0].(*ast.ASTTypeExpression)
	if assert.True(t, ok, "expected the extended type, got %T", members[0]) {
		assertTypeSource(t, iterable, "Iterable")
		assert.Equal(t, []string{"Gadget"}, typeNames(iterable.Arguments))
	}

	count, ok := members[1].(*ast.ASTDeclaration)
	if assert.True(t, ok, "expected a Declaration, got %T", members[1]) {
		if assert.Len(t, count.Names, 1) {
			assert.Equal(t, "count", count.Names[0].Name)
		}
	}

	find, ok := members[2].(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected a FuncExpression, got %T", members[2]) {
		if assert.NotNil(t, find.Name) {
			assert.Equal(t, "find", find.Name.Name)
		}
		assertTypeSource(t, find.ReturnType, "Gadget")
		assert.True(t, cog.IsNodeOfInterest(find))
	}
}

func TestTypescriptClassHeritage(t *testing.T) {
	pf := parseInline(t, "heritage.ts", "class Registry<T extends Widget> extends Base<T> implements Store, Closeable {}\n")
	if pf == nil {
		return
	}

	registry := namedFunction(t, pf, "Registry")
	if registry == nil {
		return
	}

	if assert.Len(t, registry.Arguments, 1) {
		if assert.NotNil(t, registry.Arguments[0].Identifier) {
			assert.Equal(t, "T", registry.Arguments[0].Identifier.Name)
		}
		assertTypeSource(t, registry.Arguments[0].Type, "Widget")
	}

	texprs := []*ast.ASTTypeExpression{}
	for _, chld := range registry.Block.Children() {
		if texpr, ok := chld.(*ast.ASTTypeExpression); ok {
			texprs = append(texprs, texpr)
		}
	}

	if assert.Len(t, texprs, 3) {
		assertTypeSource(t, texprs[0], "Base")
		assert.Equal(t, []string{"T"}, typeNames(texprs[0].Arguments))
		assertTypeSource(t, texprs[1], "Store")
		assertTypeSource(t, texprs[2], "Closeable")
	}
}

func TestTypescriptDestructuredDeclaration(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/typescript/baseproj/types.ts")

	decl := tsDeclarationNamed(t, pf, "firstSerial")
	if decl == nil {
		return
	}

	if assert.Len(t, decl.Virtual, 1) {
		call, ok := decl.Virtual[0].(*ast.ASTCallExpression)
		if assert.True(t, ok, "expected the constructor call, got %T", decl.Virtual[0]) && assert.NotNil(t, call.Symbol) {
			assert.Equal(t, "Gadget", call.Symbol.Name)
		}
	}
}

func TestTypescriptArrowConstDeclaration(t *testing.T) {
	pf := parseTestFile(t, "./fixtures/typescript/baseproj/types.ts")

	decl := tsDeclarationNamed(t, pf, "describe")
	if decl == nil {
		return
	}

	if assertTypeSource(t, decl.Type, "Labeler"); decl.Type != nil {
		assert.Equal(t, []string{"Gadget"}, typeNames(decl.Type.Arguments))
	}

	if assert.Len(t, decl.Virtual, 1) {
		arrow, ok := decl.Virtual[0].(*ast.ASTFuncExpression)
		if assert.True(t, ok, "expected the arrow function, got %T", decl.Virtual[0]) {
			assert.Nil(t, arrow.Name)
			assert.Len(t, arrow.Arguments, 1)
		}
	}
}

// An expression-bodied arrow narrows its block onto the expression; the
// function must stay visible to range queries or observations root at the file
func TestTypescriptArrowBlockNarrowing(t *testing.T) {
	pf := parseInline(t, "narrow.ts", "const shout = (msg: string) => msg.trim();\n")
	if pf == nil {
		return
	}

	rng, err := pf.FindSnippetRange([]byte("(msg: string) => msg.trim()"))
	if !assert.NoError(t, err) {
		return
	}

	root := pf.FindTightestEnclosingNode(rng, cog.IsNodeOfInterest)
	fn, ok := root.(*ast.ASTFuncExpression)
	if assert.True(t, ok, "expected the arrow function, got %T", root) {
		assert.Nil(t, fn.Name)
	}
}

// Every type expression spans its identifier alone and zero-width recovery
// nodes never become types
func TestTypescriptTypeExpressionSpans(t *testing.T) {
	for _, file := range []string{
		"./fixtures/typescript/baseproj/types.ts",
		"./fixtures/typescript/baseproj/generic.ts",
		"./fixtures/typescript/baseproj/method.ts",
	} {
		pf := parseTestFile(t, file)

		eachNode(pf.Module, func(node ast.ASTNode) {
			if texpr, ok := node.(*ast.ASTTypeExpression); ok {
				assert.NotEmpty(t, texpr.Name, "zero-width type expression at %v", texpr.GetPosition())
				assert.Equal(t, texpr.Name, texpr.GetStringSource(), "type expression must span the identifier alone")
			}
		})
	}
}

func TestTypescriptNormalizeDefinitionRangeNarrowsSignatures(t *testing.T) {
	src := common.NewSource(t.TempDir(), "n.d.ts", []byte(
		"    (value?: any): string;\n"+
			"    new <K, V>(entries: ReadonlyArray<[K, V]>): Map<K, V>;\n",
	))

	// A call signature reply narrows to its first parameter name
	span, err := common.NewFileRangeAutoBytePosition(src, 0, 4, 0, 26)
	if !assert.NoError(t, err) {
		return
	}

	got := languages.Typescript.NormalizeDefinitionRange(src, span)
	if assert.NotNil(t, got) {
		assert.Equal(t, "value", string(src.Buffer[got.Start.BytePosition:got.End.BytePosition]))
	}

	// A construct signature reply skips the `new` keyword no node can sit on
	span, err = common.NewFileRangeAutoBytePosition(src, 1, 4, 1, 58)
	if !assert.NoError(t, err) {
		return
	}

	got = languages.Typescript.NormalizeDefinitionRange(src, span)
	if assert.NotNil(t, got) {
		assert.Equal(t, "K", string(src.Buffer[got.Start.BytePosition:got.End.BytePosition]))
	}
}

// A signature holding nothing but keywords passes through whole: the
// member's own node spans it exactly, a keyword holds no node at all
func TestTypescriptNormalizeDefinitionRangeKeywordOnlySignature(t *testing.T) {
	src := common.NewSource(t.TempDir(), "k.d.ts", []byte("    (): void;\n"))

	span, err := common.NewFileRangeAutoBytePosition(src, 0, 4, 0, 13)
	if !assert.NoError(t, err) {
		return
	}

	assert.Same(t, span, languages.Typescript.NormalizeDefinitionRange(src, span))
}

func TestTypescriptClassifyImportType(t *testing.T) {
	workspace, err := os.Getwd()
	if !assert.NoError(t, err) {
		return
	}

	// Paths mirror what typescript-language-server actually returns: builtins
	// resolve into the lib files of the typescript package tsserver runs
	// with, Node builtins into @types/node, everything else installed into
	// node_modules
	cases := map[string]common.DependencyType{
		"../../.nvm/versions/node/v26.7.0/lib/node_modules/typescript/lib/lib.es5.d.ts": common.StandardLibraryDependency,
		"/usr/local/lib/node_modules/typescript/lib/lib.es2015.iterable.d.ts":           common.StandardLibraryDependency,
		"fixtures/typescript/multidep/node_modules/@types/node/index.d.ts":              common.StandardLibraryDependency,
		"fixtures/typescript/multidep/node_modules/left-pad/index.js":                   common.PackageDependency,
		"/x/project/node_modules/@scope/pkg/dist/index.d.ts":                            common.PackageDependency,
		"fixtures/typescript/multidep/pkg/dep1.ts":                                      common.LocalDependency,
		"/x/project/src/main.ts": common.LocalDependency,
		// node_modules directly under the workspace root arrives with no
		// leading segment at all
		"node_modules/@types/node/fs.d.ts":         common.StandardLibraryDependency,
		"node_modules/typescript/lib/lib.es5.d.ts": common.StandardLibraryDependency,
		"node_modules/lodash/index.d.ts":           common.PackageDependency,
		// pnpm stages packages under a virtual store
		"node_modules/.pnpm/typescript@5.9.3/node_modules/typescript/lib/lib.dom.d.ts": common.StandardLibraryDependency,
	}

	for path, want := range cases {
		assert.Equal(t, want, languages.Typescript.ClassifyImportType(common.NewSource(workspace, path, nil)), path)
	}
}
