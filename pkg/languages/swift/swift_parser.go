package languages_swift

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

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
// The grammar cannot parse a function without a body: function_declaration
// requires a body field, so a bodyless `public func f()` becomes an ERROR node.
// That makes .swiftinterface files -- where sourcekit-lsp resolves every
// standard library and package definition, and whose type bodies hold little
// else -- parse into whatever recovery left healthy plus the names recovered
// from ERROR nodes. Those names are what definitions resolve onto; the vertex a
// standard library definition contributes is the healthy declaration recovery
// nested it under, or the module when the name was recovered at the root.

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

func fieldNames(node parsers.ParserNode, field string) []*ast.ASTSymbol {
	var names []*ast.ASTSymbol

	node.IterateChildrenByFieldName(field, func(pn parsers.ParserNode) (bool, error) {
		names = append(names, collectPatternIdentifiers(pn)...)
		return true, nil
	})

	return names
}

// Tokens whose next sibling inside an ERROR node is a swallowed declaration's name
var declaringKeywords = map[string]bool{
	"struct":         true,
	"class":          true,
	"enum":           true,
	"protocol":       true,
	"actor":          true,
	"typealias":      true,
	"associatedtype": true,
	"func":           true,
}

// Recovery does not promise a kind for a swallowed name, so the text is what
// is checked, not the kind
func spelledLikeIdentifier(node parsers.ParserNode) bool {
	text := node.GetTextContent()

	if text == "" {
		return false
	}

	for i, r := range text {
		if r == '_' || unicode.IsLetter(r) || (i > 0 && unicode.IsDigit(r)) {
			continue
		}

		return false
	}

	return true
}

// An identifier after `->`, `:` or a modifier is a reference, so the keyword
// is what tells a declared name from a use
func declaredAfterKeyword(node parsers.ParserNode, prevKind string) (*ast.ASTSymbol, bool) {
	if !declaringKeywords[prevKind] || !hasSourceWidth(node) || !spelledLikeIdentifier(node) {
		return nil, false
	}

	return ast.NewASTSymbol(ast.NewASTNodeContainer(node), node.GetTextContent()), true
}

// recoveredNames collects the names an ERROR node still declares: whatever
// recovery filed under the name field, plus every bare identifier that
// follows a declaring keyword
func recoveredNames(node parsers.ParserNode) []*ast.ASTSymbol {
	var names []*ast.ASTSymbol
	prevKind := ""

	node.IterateAllChildren(func(child parsers.ParserNode, field string) (bool, error) {
		var filed []*ast.ASTSymbol

		if field == "name" {
			filed = collectPatternIdentifiers(child)
		}

		if len(filed) == 0 {
			if name, ok := declaredAfterKeyword(child, prevKind); ok {
				filed = append(filed, name)
			}
		}

		names = append(names, filed...)
		prevKind = child.Kind

		return true, nil
	})

	// A swallowed `init` becomes an ERROR node spanning exactly the keyword,
	// which is where constructor calls resolve, so it stands in for the name
	if node.GetTextContent() == "init" {
		names = append(names, ast.NewASTSymbol(ast.NewASTNodeContainer(node), "init"))
	}

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

// typeExpression maps a type onto the identifiers definitions resolve to.
// Nameless composites (optionals, dictionaries, function types) fold flat:
// the first named type inside is the head and the rest are its arguments.
func typeExpression(node parsers.ParserNode) *ast.ASTTypeExpression {
	types := collectTypes(node)

	if len(types) == 0 {
		return nil
	}

	types[0].Arguments = append(types[0].Arguments, types[1:]...)

	return types[0]
}

// An attribute (`@MainActor`) spells its name as a user_type but never
// denotes one, so it contributes nothing
func collectTypes(node parsers.ParserNode) []*ast.ASTTypeExpression {
	switch node.Kind {
	case "type_identifier":
		if !hasSourceWidth(node) {
			return nil
		}

		return []*ast.ASTTypeExpression{ast.NewASTTypeExpression(ast.NewASTNodeContainer(node), node.GetTextContent())}

	case "user_type":
		if texpr := userTypeExpression(node); texpr != nil {
			return []*ast.ASTTypeExpression{texpr}
		}

		return nil

	case "type_modifiers", "attribute":
		return nil
	}

	var types []*ast.ASTTypeExpression

	node.IterateChildren(func(child parsers.ParserNode) (bool, error) {
		types = append(types, collectTypes(child)...)
		return true, nil
	})

	return types
}

// userTypeExpression maps a dotted, possibly generic user_type. The head is
// the last segment, except that a trailing Type or Protocol is the metatype
// marker and is dropped (Gadget.Type is about Gadget).
func userTypeExpression(node parsers.ParserNode) *ast.ASTTypeExpression {
	var segments []parsers.ParserNode
	var args []*ast.ASTTypeExpression

	node.IterateChildren(func(child parsers.ParserNode) (bool, error) {
		switch child.Kind {
		case "type_identifier":
			if hasSourceWidth(child) {
				segments = append(segments, child)
			}

		case "type_arguments":
			child.IterateChildren(func(arg parsers.ParserNode) (bool, error) {
				if texpr := typeExpression(arg); texpr != nil {
					args = append(args, texpr)
				}
				return true, nil
			})
		}

		return true, nil
	})

	if len(segments) > 1 {
		if marker := segments[len(segments)-1].GetTextContent(); marker == "Type" || marker == "Protocol" {
			segments = segments[:len(segments)-1]
		}
	}

	if len(segments) == 0 {
		return nil
	}

	head := segments[len(segments)-1]
	texpr := ast.NewASTTypeExpression(ast.NewASTNodeContainer(head), head.GetTextContent())
	texpr.Arguments = append(texpr.Arguments, args...)

	if len(segments) > 1 {
		qual := segments[len(segments)-2]
		texpr.Namespace = ast.NewASTSymbol(ast.NewASTNodeContainer(qual), qual.GetTextContent())
	}

	return texpr
}

// The grammar files an attributed type's `@MainActor` under the type field and
// the type itself as its next sibling, so type_modifiers is stepped over
func annotatedType(node parsers.ParserNode, field string) *ast.ASTTypeExpression {
	typeNode, ok := node.ChildByFieldName(field)

	if !ok {
		return nil
	}

	if typeNode.Kind == "type_modifiers" {
		if typeNode, ok = typeNode.NextNamedSibling(); !ok {
			return nil
		}
	}

	return typeExpression(typeNode)
}

func bindingType(node parsers.ParserNode) *ast.ASTTypeExpression {
	annotation, ok := node.GetNthChildByKind("type_annotation", 0)

	if !ok {
		return nil
	}

	return annotatedType(annotation, "type")
}

func appendArgument(fn *ast.ASTFuncExpression, pn parsers.ParserNode) {
	var idSym *ast.ASTSymbol

	// The name field also accepts types in this grammar, so the kind is checked
	// rather than trusted
	if nameNode, ok := pn.ChildByFieldName("name"); ok && nameNode.Kind == "simple_identifier" && hasSourceWidth(nameNode) {
		idSym = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
	}

	fn.Arguments = append(fn.Arguments, ast.NewASTFuncArgument(ast.NewASTNodeContainer(pn), idSym, annotatedType(pn, "type")))
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

func collectTypeParameters(node parsers.ParserNode, fn *ast.ASTFuncExpression) {
	params, ok := node.GetNthChildByKind("type_parameters", 0)

	if !ok {
		return
	}

	params.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
		if pn.Kind != "type_parameter" {
			return true, nil
		}

		nameNode, ok := pn.GetNthChildByKind("type_identifier", 0)

		if !ok || !hasSourceWidth(nameNode) {
			return true, nil
		}

		idSym := ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
		fn.Arguments = append(fn.Arguments, ast.NewASTFuncArgument(ast.NewASTNodeContainer(pn), idSym, annotatedType(pn, "name")))

		return true, nil
	})
}

func typeParameterNames(node parsers.ParserNode) []*ast.ASTSymbol {
	params, ok := node.GetNthChildByKind("type_parameters", 0)

	if !ok {
		return nil
	}

	var names []*ast.ASTSymbol

	params.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
		if pn.Kind != "type_parameter" {
			return true, nil
		}

		if nameNode, ok := pn.GetNthChildByKind("type_identifier", 0); ok && hasSourceWidth(nameNode) {
			names = append(names, ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent()))
		}

		return true, nil
	})

	return names
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
	fn.ReturnType = annotatedType(node, "return_type")

	collectTypeParameters(node, fn)
	collectParameters(node, fn)

	if err := trx.Emit(fn); err != nil {
		return true, err
	}

	return true, trx.WalkChildrenInto(ctx, fn)
}

// emitDeclaration emits an ASTDeclaration carrying the given names, used for
// the declaration forms that have no body to narrow a function block down to.
// A node that declares nothing is walked in place instead.
func emitDeclaration(ctx context.Context, trx *parsers.TransformContext, node parsers.ParserNode, names []*ast.ASTSymbol, typeExpr *ast.ASTTypeExpression) error {
	if len(names) == 0 {
		return trx.WalkChildren(ctx)
	}

	decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(node))
	decl.Names = names
	decl.Type = typeExpr

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

		fn.ReturnType = annotatedType(node, "return_type")

		collectTypeParameters(node, fn)
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

		collectTypeParameters(node, fn)

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
		return emitDeclaration(ctx, trx, node, fieldNames(node, "name"), bindingType(node))

	case "typealias_declaration", "associatedtype_declaration":
		// A member-less named type needs only a declaration on its name, with
		// the aliased type, constraint and default hanging off it as its type
		nameNode, ok := node.ChildByFieldName("name")

		if !ok || nameNode.Kind != "type_identifier" || !hasSourceWidth(nameNode) {
			return trx.WalkChildren(ctx)
		}

		names := []*ast.ASTSymbol{ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())}
		names = append(names, typeParameterNames(node)...)

		var types []*ast.ASTTypeExpression

		for _, field := range []string{"value", "must_inherit", "default_value"} {
			if texpr := annotatedType(node, field); texpr != nil {
				types = append(types, texpr)
			}
		}

		var declared *ast.ASTTypeExpression

		if len(types) > 0 {
			declared = types[0]
			declared.Arguments = append(declared.Arguments, types[1:]...)
		}

		return emitDeclaration(ctx, trx, node, names, declared)

	case "enum_entry":
		// Cases with associated values are callable, so definitions resolve here
		return emitDeclaration(ctx, trx, node, fieldNames(node, "name"), nil)

	case "protocol_function_declaration":
		if emitted, err := emitBodylessFunction(ctx, trx, node); emitted {
			return err
		}

		// Operator requirements name themselves with something other than a
		// plain identifier; those still declare a name worth resolving onto
		return emitDeclaration(ctx, trx, node, fieldNames(node, "name"), nil)

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
		// protocol, so `public func f() -> Int` -- most of what a .swiftinterface
		// holds inside its type bodies, and any half-typed one being edited --
		// lands here.
		// Recovery keeps the name field, and the return type is spelled
		// user_type rather than simple_identifier, so reading the name picks up
		// the declaration and not its type. Without this the name is dropped and
		// a definition resolving onto it finds nothing.
		//
		// A Declaration and not a function: recovery groups whatever it could
		// not parse, so an ERROR spans an arbitrary run of declarations rather
		// than one. Reading a function out of that shape guesses at structure
		// the grammar never established. The names are kept, so a definition
		// still resolves; the enclosing vertex is the module.
		return emitDeclaration(ctx, trx, node, recoveredNames(node), nil)

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

	rootNode := parsers.NewParserNode(src, tree.RootNode())
	root := ast.NewASTModule(ast.NewASTNodeContainer(rootNode), name)

	// Recovery often makes a generated interface one ERROR node that is the
	// root itself, and the walk never visits the root, so the names it declares
	// directly (`struct Int`, `struct String`) are read here
	if rootNode.Kind == "ERROR" {
		if names := recoveredNames(rootNode); len(names) > 0 {
			decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(rootNode))
			decl.Names = names
			root.AppendChild(decl)
		}
	}

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
