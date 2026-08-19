package languages_swift

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	tree_sitter_swift "github.com/alex-pinkus/tree-sitter-swift/bindings/go"
	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/lsp"
	"github.com/schahriar/captn/pkg/parsers"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// Two things about Swift are worth knowing before changing this file.
//
// Types are functions here. class, struct, enum, actor, extension and protocol
// all map to ASTFuncExpression, because only FuncExpression and Module become
// observation-graph vertices and a Swift type is where its members live.
//
// The grammar cannot parse a declaration without a body: function_declaration
// requires a body field, so a bodyless `public func f()` becomes an ERROR node.
// That makes .swiftinterface files -- which are nothing but bodyless
// declarations -- parse into little more than their module. It costs nothing
// today because SearchSnippet keeps only local dependencies and every generated
// interface classifies as stdlib or package, but a caller that parses one
// directly should expect a module and not much else.

// hasSourceWidth reports whether a node spans any bytes; tree-sitter error
// recovery produces zero-width nodes that must never become AST nodes (the
// interval index rejects empty ranges)
func hasSourceWidth(node parsers.ParserNode) bool {
	return node.Range.StartByte < node.Range.EndByte
}

func firstNamedChild(node parsers.ParserNode) (parsers.ParserNode, bool) {
	var first parsers.ParserNode
	found := false

	node.IterateChildren(func(child parsers.ParserNode) (bool, error) {
		first = child
		found = true
		return false, nil
	})

	return first, found
}

// collectPatternIdentifiers collects the names a Swift binding pattern
// declares. LSP definitions land on these, so every bound name needs a symbol.
// Only simple_identifier is collected: the grammar spells types as
// type_identifier, so recursing cannot mistake a type annotation for a binding.
func collectPatternIdentifiers(node parsers.ParserNode) []*ast.ASTSymbol {
	if node.Kind == "simple_identifier" {
		if !hasSourceWidth(node) {
			return nil
		}

		return []*ast.ASTSymbol{ast.NewASTSymbol(ast.NewASTNodeContainer(node), node.GetTextContent())}
	}

	var names []*ast.ASTSymbol

	node.IterateChildren(func(child parsers.ParserNode) (bool, error) {
		names = append(names, collectPatternIdentifiers(child)...)
		return true, nil
	})

	return names
}

// emitBindingDeclaration emits an ASTDeclaration over a binding target (loop
// item, optional binding, catch alias) so LSP-resolved definition ranges always
// contain a symbol
func emitBindingDeclaration(trx *parsers.TransformContext, target parsers.ParserNode) error {
	names := collectPatternIdentifiers(target)

	if len(names) == 0 {
		return nil
	}

	decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(target))
	decl.Names = names

	return trx.Emit(decl)
}

func appendArgument(fn *ast.ASTFuncExpression, pn parsers.ParserNode) {
	var idSym *ast.ASTSymbol
	var typeSym *ast.ASTSymbol

	// The name field also accepts types in this grammar, so the kind is checked
	// rather than trusted
	if nameNode, ok := pn.ChildByFieldName("name"); ok && nameNode.Kind == "simple_identifier" {
		idSym = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
	}

	if typeNode, ok := pn.ChildByFieldName("type"); ok {
		typeSym = ast.NewASTSymbol(ast.NewASTNodeContainer(typeNode), typeNode.GetTextContent())
	}

	fn.Arguments = append(fn.Arguments, ast.NewASTFuncArgument(ast.NewASTNodeContainer(pn), idSym, typeSym))
}

// collectParameters reads the parameters of a function-like declaration. Swift
// hangs them directly off the declaration, so unlike Go and Python there is no
// parameter list node and no dependency on the parent set by an earlier case.
func collectParameters(node parsers.ParserNode, fn *ast.ASTFuncExpression) {
	node.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
		if pn.Kind == "parameter" {
			appendArgument(fn, pn)
		}

		return true, nil
	})
}

func collectLambdaParameters(typeNode parsers.ParserNode, fn *ast.ASTFuncExpression) {
	params, ok := typeNode.GetNthChildByKind("lambda_function_type_parameters", 0)

	if !ok {
		return
	}

	params.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
		if pn.Kind == "lambda_parameter" {
			appendArgument(fn, pn)
		}

		return true, nil
	})
}

// emitBodylessFunction emits a function that the grammar gave no body: a
// protocol requirement, or a declaration recovered from an ERROR node. It has to
// be a FuncExpression rather than a Declaration because only FuncExpression and
// Module become observation-graph vertices, and collapsing a standard library
// function onto its module answers "what does this module do" when the question
// was about one function.
//
// The auto-created block is left spanning the whole declaration, so the block
// shadows the function in the interval index. That is deliberate and harmless
// here: a definition resolves onto the name, the name keeps its own range, and
// walking up from it reaches this function. There is no body for a range query
// to land inside of.
func emitBodylessFunction(ctx context.Context, trx *parsers.TransformContext, node parsers.ParserNode) (bool, error) {
	nameNode, ok := node.ChildByFieldName("name")

	if !ok || nameNode.Kind != "simple_identifier" || !hasSourceWidth(nameNode) {
		return false, nil
	}

	fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))
	fn.Name = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())

	collectParameters(node, fn)

	if err := trx.Emit(fn); err != nil {
		return true, err
	}

	return true, trx.WalkChildrenInto(ctx, fn)
}

// emitNamedDeclaration emits an ASTDeclaration carrying every name found under
// the repeated field, used for the declaration forms that have no body to
// narrow a function block down to
func emitNamedDeclaration(ctx context.Context, trx *parsers.TransformContext, node parsers.ParserNode, field string) error {
	var names []*ast.ASTSymbol

	if err := node.IterateChildrenByFieldName(field, func(pn parsers.ParserNode) (bool, error) {
		names = append(names, collectPatternIdentifiers(pn)...)
		return true, nil
	}); err != nil {
		return err
	}

	if len(names) == 0 {
		return trx.WalkChildren(ctx)
	}

	decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(node))
	decl.Names = names

	if err := trx.Emit(decl); err != nil {
		return err
	}

	// Walk into the declaration so calls on the right-hand side land in Virtual
	return trx.WalkChildrenInto(ctx, decl)
}

func SwiftTransformer(ctx context.Context, trx *parsers.TransformContext, node parsers.ParserNode) error {
	switch node.Kind {
	case "import_declaration":
		// The module path is the only LSP-resolvable text; anchoring on the
		// declaration would put the range on the `import` keyword
		pathNode, ok := node.GetNthChildByKind("identifier", 0)

		if !ok {
			return nil
		}

		imp := ast.NewASTImportStatement(ast.NewASTNodeContainer(pathNode))
		imp.Reference = ast.NewASTSymbol(ast.NewASTNodeContainer(pathNode), pathNode.GetTextContent())

		return trx.Emit(imp)

	case "function_declaration", "init_declaration", "deinit_declaration":
		fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))

		// init_declaration names itself with the `init` keyword; definitions of
		// constructor calls resolve onto it
		if nameNode, ok := node.ChildByFieldName("name"); ok && hasSourceWidth(nameNode) {
			fn.Name = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
		}

		if retNode, ok := node.ChildByFieldName("return_type"); ok {
			fn.ReturnType = ast.NewASTSymbol(ast.NewASTNodeContainer(retNode), retNode.GetTextContent())
		}

		collectParameters(node, fn)

		// Functions auto-assign a block spanning the whole declaration; narrow
		// it to the body so the function stays visible to range queries
		if bodyNode, ok := node.ChildByFieldName("body"); ok && hasSourceWidth(bodyNode) {
			fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(bodyNode))
		}

		if err := trx.Emit(fn); err != nil {
			return err
		}

		return trx.WalkChildrenInto(ctx, fn)

	case "class_declaration", "protocol_declaration":
		// class, struct, enum, actor, extension and protocol all map to
		// FuncExpression on purpose: they are callable or constructible, they
		// carry the members observations attach to, and they become
		// observation-graph vertices like any function
		fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))

		if nameNode, ok := node.ChildByFieldName("name"); ok && hasSourceWidth(nameNode) {
			fn.Name = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
		}

		if bodyNode, ok := node.ChildByFieldName("body"); ok && hasSourceWidth(bodyNode) {
			fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(bodyNode))
		}

		if err := trx.Emit(fn); err != nil {
			return err
		}

		return trx.WalkChildrenInto(ctx, fn)

	case "lambda_literal":
		fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))

		if typeNode, ok := node.ChildByFieldName("type"); ok {
			collectLambdaParameters(typeNode, fn)
		}

		// The literal's own range includes the braces; narrowing to the
		// statement list keeps the closure visible to range queries
		if stmts, ok := node.GetNthChildByKind("statements", 0); ok && hasSourceWidth(stmts) {
			fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(stmts))
		}

		if err := trx.Emit(fn); err != nil {
			return err
		}

		return trx.WalkChildrenInto(ctx, fn)

	case "property_declaration", "protocol_property_declaration":
		return emitNamedDeclaration(ctx, trx, node, "name")

	case "enum_entry":
		// Cases with associated values are callable, so definitions resolve here
		return emitNamedDeclaration(ctx, trx, node, "name")

	case "protocol_function_declaration":
		if emitted, err := emitBodylessFunction(ctx, trx, node); emitted {
			return err
		}

		// Operator requirements name themselves with something other than a
		// plain identifier; those still declare a name worth resolving onto
		return emitNamedDeclaration(ctx, trx, node, "name")

	case "for_statement":
		if itemNode, ok := node.ChildByFieldName("item"); ok {
			if err := emitBindingDeclaration(trx, itemNode); err != nil {
				return err
			}
		}

		return trx.WalkChildren(ctx)

	case "if_statement", "guard_statement", "while_statement":
		// `if let x = …` optional bindings; the grammar exposes each bound name
		// as a repeated field on the statement itself
		if err := node.IterateChildrenByFieldName("bound_identifier", func(pn parsers.ParserNode) (bool, error) {
			if err := emitBindingDeclaration(trx, pn); err != nil {
				return false, err
			}

			return true, nil
		}); err != nil {
			return err
		}

		return trx.WalkChildren(ctx)

	case "catch_block":
		if errNode, ok := node.ChildByFieldName("error"); ok {
			if err := emitBindingDeclaration(trx, errNode); err != nil {
				return err
			}
		}

		return trx.WalkChildren(ctx)

	case "call_expression":
		callExpr := ast.NewASTCallExpression(ast.NewASTNodeContainer(node))

		if calleeNode, ok := firstNamedChild(node); ok {
			switch calleeNode.Kind {
			case "navigation_expression":
				// Skips over chained calls (e.g. x.y().z()), which are handled
				// below at WalkChildrenInto
				if target, ok := calleeNode.ChildByFieldName("target"); ok && target.Kind == "simple_identifier" {
					callExpr.Namespace = ast.NewASTSymbol(ast.NewASTNodeContainer(target), target.GetTextContent())
				}

				if suffix, ok := calleeNode.ChildByFieldName("suffix"); ok {
					if name, ok := suffix.ChildByFieldName("suffix"); ok && name.Kind == "simple_identifier" {
						callExpr.Symbol = ast.NewASTSymbol(ast.NewASTNodeContainer(name), name.GetTextContent())
					}
				}

			case "simple_identifier":
				callExpr.Symbol = ast.NewASTSymbol(ast.NewASTNodeContainer(calleeNode), calleeNode.GetTextContent())
			}
		}

		// A call without an identifiable callee cannot resolve through the LSP;
		// its children still index under the enclosing block
		if callExpr.Symbol == nil {
			return trx.WalkChildren(ctx)
		}

		if err := trx.Emit(callExpr); err != nil {
			return err
		}

		// Handles chained and nested calls, and trailing closures
		return trx.WalkChildrenInto(ctx, callExpr)

	case "ERROR":
		// The grammar has no production for a function without a body outside a
		// protocol, so `public func f() -> Int` -- every declaration in a
		// .swiftinterface, and any half-typed one being edited -- lands here.
		// Recovery keeps the name field, and the return type is spelled
		// user_type rather than simple_identifier, so reading the name picks up
		// the declaration and not its type. Without this the name is dropped and
		// a definition resolving onto it finds nothing.
		//
		// A Declaration and not a function: recovery groups whatever it could
		// not parse, so an ERROR spans an arbitrary run of declarations rather
		// than one. Reading a function out of that shape guesses at structure
		// the grammar never established. The name is kept, so a definition still
		// resolves; the enclosing vertex is the module.
		return emitNamedDeclaration(ctx, trx, node, "name")

	default:
		return trx.WalkChildren(ctx)
	}
}

type SwiftLanguageSupportDefinition struct{}

func NewSwiftLanguageSupportDefinition() *SwiftLanguageSupportDefinition {
	return &SwiftLanguageSupportDefinition{}
}

var (
	// Swift has no source for a compiled module. sourcekit-lsp answers every
	// definition that leaves the current module by writing a generated
	// interface into a temporary directory and pointing at that file, so this
	// is where imports and standard library calls actually resolve to.
	generatedInterfaceRE = regexp.MustCompile(`/sourcekit-lsp/GeneratedInterfaces/`)
	// Generated interfaces are named for their module, which is the only thing
	// distinguishing the standard library from anything else once resolved
	stdlibInterfaceRE = regexp.MustCompile(`/Swift(\.[^/]*)?\.swiftinterface$`)
	// SwiftPM and Xcode both stage resolved packages as checkouts
	swiftCheckoutsRE = regexp.MustCompile(`/(\.build|SourcePackages)/checkouts/`)
	derivedDataRE    = regexp.MustCompile(`/DerivedData/`)
	// Definitions that land in a shipped toolchain or SDK rather than in a
	// generated interface: Xcode keeps both under the app bundle, swiftly and
	// the swift.org tarballs keep the toolchain under usr/lib/swift
	toolchainRE = regexp.MustCompile(`(\.xctoolchain/|/Developer/SDKs/|/usr/lib/swift(_static)?/|/usr/share/swift/|/\.swiftly/toolchains/)`)
)

func (slsd *SwiftLanguageSupportDefinition) ClassifyImportType(s *common.Source) common.DependencyType {
	p := filepath.ToSlash(filepath.Clean(s.Path))

	// A local target's own interface is written next to the standard library's
	// and cannot be told apart by path. Treating them all as the module
	// boundary they are is correct for traversal: captn reaches a local Swift
	// file through the definition of a call, which resolves to real source,
	// never through the module import.
	if generatedInterfaceRE.MatchString(p) {
		if stdlibInterfaceRE.MatchString(p) {
			return common.StandardLibraryDependency
		}

		return common.PackageDependency
	}

	// Checked before the toolchain patterns: a checkout can live anywhere,
	// including under DerivedData inside the Xcode app support directories
	if swiftCheckoutsRE.MatchString(p) || derivedDataRE.MatchString(p) {
		return common.PackageDependency
	}

	if toolchainRE.MatchString(p) {
		return common.StandardLibraryDependency
	}

	return common.LocalDependency
}

func (slsd *SwiftLanguageSupportDefinition) GetLSPServerRequirement() lsp.ServerRequirement {
	// banstructlit:ignore
	return lsp.ServerRequirement{
		Name:           sourcekitName,
		InstallCommand: sourcekitInstallCommand(),
		Locate:         sourcekitPath,
	}
}

func (slsd *SwiftLanguageSupportDefinition) NewLSPServer(ctx context.Context) (*lsp.ServerProcess, error) {
	execPath, err := sourcekitPath(ctx)

	if err != nil {
		return nil, err
	}

	// sourcekit-lsp speaks stdio with no flag; its logs go to stderr already
	cmd := exec.CommandContext(ctx, execPath)
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	return lsp.NewServerProcess(stdout, stdin, cmd.Wait, func() error {
		return cmd.Process.Kill()
	}), nil
}

func (slsd *SwiftLanguageSupportDefinition) Parse(ctx context.Context, src *common.Source, tree *tree_sitter.Tree) (*ast.ASTModule, error) {
	// Swift has no per-file package clause; the file name is the only name a
	// single file carries
	name := strings.TrimSuffix(filepath.Base(src.Path), filepath.Ext(src.Path))

	root := ast.NewASTModule(ast.NewASTNodeContainer(
		parsers.NewParserNode(src, tree.RootNode()),
	), name)

	if err := parsers.WalkTransformTree(ctx, src, tree, root, SwiftTransformer); err != nil {
		return nil, err
	}

	return root, nil
}

func (slsd *SwiftLanguageSupportDefinition) GetLanguageID() string {
	return "swift"
}

func (slsd *SwiftLanguageSupportDefinition) GetTreeSitterLanguage() *tree_sitter.Language {
	return tree_sitter.NewLanguage(tree_sitter_swift.Language())
}
