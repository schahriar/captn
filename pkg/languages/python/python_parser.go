package languages_python

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/lsp"
	"github.com/schahriar/captn/pkg/parsers"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
)

// emitImportedName emits one ASTImportStatement for a dotted_name or
// aliased_import node. The emitted node's range starts at the module path so
// LSP definition lookups land on resolvable text, never on the import keyword.
func emitImportedName(trx *parsers.TransformContext, node parsers.ParserNode, module string) error {
	imp := ast.NewASTImportStatement(ast.NewASTNodeContainer(node))

	nameNode := node
	if node.Kind == "aliased_import" {
		if n, ok := node.ChildByFieldName("name"); ok {
			nameNode = n
		}
		if aliasNode, ok := node.ChildByFieldName("alias"); ok {
			imp.Namespace = ast.NewASTSymbol(ast.NewASTNodeContainer(aliasNode), aliasNode.GetTextContent())
		}
	}

	reference := nameNode.GetTextContent()
	if module != "" {
		if strings.HasSuffix(module, ".") {
			reference = module + reference
		} else {
			reference = module + "." + reference
		}
	}
	imp.Reference = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), reference)

	return trx.Emit(imp)
}

// parameterIdentifier finds the identifier a parameter binds, if any
func parameterIdentifier(node parsers.ParserNode) (parsers.ParserNode, bool) {
	switch node.Kind {
	case "identifier":
		return node, true

	case "default_parameter", "typed_default_parameter":
		if nameNode, ok := node.ChildByFieldName("name"); ok && nameNode.Kind == "identifier" {
			return nameNode, true
		}

	case "typed_parameter", "list_splat_pattern", "dictionary_splat_pattern":
		var id parsers.ParserNode
		found := false
		node.IterateChildren(func(child parsers.ParserNode) (bool, error) {
			if child.Kind == "type" {
				return true, nil
			}
			if idNode, ok := parameterIdentifier(child); ok {
				id = idNode
				found = true
				return false, nil
			}
			return true, nil
		})
		return id, found
	}

	var zero parsers.ParserNode
	return zero, false
}

// collectBindingTargets collects the identifiers a binding target declares.
// LSP definitions land on these, so every bound identifier needs a symbol:
// attribute targets declare their attribute name (`self.cb = f` declares cb);
// subscript targets declare nothing nameable.
func collectBindingTargets(node parsers.ParserNode) []*ast.ASTSymbol {
	switch node.Kind {
	case "identifier":
		// Error recovery inserts zero-width MISSING identifiers into
		// half-typed targets; they must never become AST nodes
		if !hasSourceWidth(node) {
			return nil
		}
		return []*ast.ASTSymbol{ast.NewASTSymbol(ast.NewASTNodeContainer(node), node.GetTextContent())}

	case "attribute":
		if attr, ok := node.ChildByFieldName("attribute"); ok && hasSourceWidth(attr) {
			return []*ast.ASTSymbol{ast.NewASTSymbol(ast.NewASTNodeContainer(attr), attr.GetTextContent())}
		}

	case "pattern_list", "tuple_pattern", "list_pattern", "list_splat_pattern", "as_pattern_target":
		var names []*ast.ASTSymbol
		node.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
			names = append(names, collectBindingTargets(pn)...)
			return true, nil
		})
		return names
	}

	return nil
}

// emitBindingDeclaration emits an ASTDeclaration on a binding target node
// (loop variable, walrus name, with/except alias) so LSP-resolved definition
// ranges always contain a symbol
func emitBindingDeclaration(trx *parsers.TransformContext, target parsers.ParserNode) error {
	names := collectBindingTargets(target)

	if len(names) == 0 {
		return nil
	}

	decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(target))
	decl.Names = names

	return trx.Emit(decl)
}

// hasSourceWidth reports whether a node spans any bytes; tree-sitter error
// recovery produces zero-width nodes that must never become AST nodes (the
// interval index rejects empty ranges)
func hasSourceWidth(node parsers.ParserNode) bool {
	return node.Range.StartByte < node.Range.EndByte
}

// typeExpression maps an annotation onto the identifier a definition resolves
// to. The first named type inside is the head and the rest are its arguments.
func typeExpression(node parsers.ParserNode) *ast.ASTTypeExpression {
	return foldTypes(collectTypes(node))
}

func foldTypes(types []*ast.ASTTypeExpression) *ast.ASTTypeExpression {
	if len(types) == 0 {
		return nil
	}

	types[0].Arguments = append(types[0].Arguments, types[1:]...)

	return types[0]
}

func collectTypes(node parsers.ParserNode) []*ast.ASTTypeExpression {
	switch node.Kind {
	case "identifier":
		if !hasSourceWidth(node) {
			return nil
		}

		return []*ast.ASTTypeExpression{ast.NewASTTypeExpression(ast.NewASTNodeContainer(node), node.GetTextContent())}

	case "attribute":
		attr, ok := node.ChildByFieldName("attribute")

		if !ok || !hasSourceWidth(attr) {
			return nil
		}

		texpr := ast.NewASTTypeExpression(ast.NewASTNodeContainer(attr), attr.GetTextContent())

		if object, ok := node.ChildByFieldName("object"); ok && object.Kind == "identifier" {
			texpr.Namespace = ast.NewASTSymbol(ast.NewASTNodeContainer(object), object.GetTextContent())
		}

		return []*ast.ASTTypeExpression{texpr}

	case "call":
		// `Annotated[int, Field(gt=0)]` carries metadata, not types
		return nil
	}

	var types []*ast.ASTTypeExpression

	node.IterateChildren(func(child parsers.ParserNode) (bool, error) {
		types = append(types, collectTypes(child)...)
		return true, nil
	})

	if node.Kind == "generic_type" || node.Kind == "subscript" {
		if head := foldTypes(types); head != nil {
			return []*ast.ASTTypeExpression{head}
		}

		return nil
	}

	return types
}

func typeParameters(node parsers.ParserNode) []*ast.ASTFuncArgument {
	var args []*ast.ASTFuncArgument

	node.IterateChildren(func(param parsers.ParserNode) (bool, error) {
		if param.Kind != "type" {
			return true, nil
		}

		nameNode, ok := typeParameterName(param)

		if !ok || !hasSourceWidth(nameNode) {
			return true, nil
		}

		name := ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())

		var bound *ast.ASTTypeExpression

		if constrained, ok := param.GetNthChildByKind("constrained_type", 0); ok {
			if boundNode, ok := constrained.GetNthChildByKind("type", 1); ok {
				bound = typeExpression(boundNode)
			}
		}

		args = append(args, ast.NewASTFuncArgument(ast.NewASTNodeContainer(param), name, bound))

		return true, nil
	})

	return args
}

// A constrained `T: int` spells the name as its first type; the second is the bound
func typeParameterName(param parsers.ParserNode) (parsers.ParserNode, bool) {
	if inner, ok := param.GetNthChildByKind("constrained_type", 0); ok {
		if first, ok := inner.GetNthChildByKind("type", 0); ok {
			return first.GetNthChildByKind("identifier", 0)
		}
	}

	if inner, ok := param.GetNthChildByKind("splat_type", 0); ok {
		return inner.GetNthChildByKind("identifier", 0)
	}

	return param.GetNthChildByKind("identifier", 0)
}

func aliasNames(left parsers.ParserNode) []*ast.ASTSymbol {
	head := left

	if generic, ok := left.GetNthChildByKind("generic_type", 0); ok {
		head = generic
	}

	var names []*ast.ASTSymbol

	if id, ok := head.GetNthChildByKind("identifier", 0); ok && hasSourceWidth(id) {
		names = append(names, ast.NewASTSymbol(ast.NewASTNodeContainer(id), id.GetTextContent()))
	}

	if params, ok := head.GetNthChildByKind("type_parameter", 0); ok {
		params.IterateChildren(func(param parsers.ParserNode) (bool, error) {
			if id, ok := typeParameterName(param); ok && hasSourceWidth(id) {
				names = append(names, ast.NewASTSymbol(ast.NewASTNodeContainer(id), id.GetTextContent()))
			}
			return true, nil
		})
	}

	return names
}

func PythonTransformer(ctx context.Context, trx *parsers.TransformContext, node parsers.ParserNode) error {
	switch node.Kind {
	case "import_statement":
		return node.IterateChildrenByFieldName("name", func(pn parsers.ParserNode) (bool, error) {
			if err := emitImportedName(trx, pn, ""); err != nil {
				return false, err
			}
			return true, nil
		})

	case "import_from_statement", "future_import_statement":
		module := ""
		var moduleNode parsers.ParserNode
		hasModule := false

		if mn, ok := node.ChildByFieldName("module_name"); ok {
			module = mn.GetTextContent()
			moduleNode = mn
			hasModule = true
		}

		emitted := false
		err := node.IterateChildrenByFieldName("name", func(pn parsers.ParserNode) (bool, error) {
			emitted = true
			if err := emitImportedName(trx, pn, module); err != nil {
				return false, err
			}
			return true, nil
		})

		if err != nil {
			return err
		}

		// Wildcard imports carry no name fields; the module path itself is
		// the only resolvable text. For relative modules that is the
		// dotted_name inside the relative_import.
		if !emitted && hasModule {
			switch moduleNode.Kind {
			case "dotted_name":
				return emitImportedName(trx, moduleNode, "")
			case "relative_import":
				if dn, ok := moduleNode.GetNthChildByKind("dotted_name", 0); ok {
					return emitImportedName(trx, dn, "")
				}
			}
		}

		return nil

	case "function_definition", "class_definition":
		// Classes map to FuncExpression on purpose: they are callable, LSP
		// definitions of constructor calls land on the class name, and the
		// class becomes an observation vertex like any function
		fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))

		if nameNode, ok := node.ChildByFieldName("name"); ok {
			fn.Name = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
		}

		if paramsNode, ok := node.ChildByFieldName("type_parameters"); ok {
			fn.Arguments = append(fn.Arguments, typeParameters(paramsNode)...)
		}

		if retNode, ok := node.ChildByFieldName("return_type"); ok {
			fn.ReturnType = typeExpression(retNode)
		}

		// Functions auto-assign a block but we want to narrow down the block
		// to the body so the function stays visible to range queries
		if bodyNode, ok := node.ChildByFieldName("body"); ok && hasSourceWidth(bodyNode) {
			fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(bodyNode))
		}

		if err := trx.Emit(fn); err != nil {
			return err
		}

		return trx.WalkChildrenInto(ctx, fn)

	case "lambda":
		// The anonymous "lambda" keyword token shares this Kind; only the
		// expression node carries a body field
		bodyNode, ok := node.ChildByFieldName("body")
		if !ok {
			return trx.WalkChildren(ctx)
		}

		fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))

		if hasSourceWidth(bodyNode) {
			fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(bodyNode))
		}

		if err := trx.Emit(fn); err != nil {
			return err
		}

		return trx.WalkChildrenInto(ctx, fn)

	case "parameters", "lambda_parameters":
		if fn, ok := trx.Parent.(*ast.ASTFuncExpression); ok {
			node.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
				idNode, ok := parameterIdentifier(pn)
				if !ok {
					return true, nil
				}

				idSym := ast.NewASTSymbol(ast.NewASTNodeContainer(idNode), idNode.GetTextContent())

				var typeExpr *ast.ASTTypeExpression
				if typeNode, ok := pn.ChildByFieldName("type"); ok {
					typeExpr = typeExpression(typeNode)
				}

				fn.Arguments = append(fn.Arguments, ast.NewASTFuncArgument(ast.NewASTNodeContainer(pn), idSym, typeExpr))

				return true, nil
			})
		}

		// Default values may hold calls or lambdas; keep walking so they
		// land in the enclosing function's block
		return trx.WalkChildren(ctx)

	case "assignment":
		var names []*ast.ASTSymbol
		if leftNode, ok := node.ChildByFieldName("left"); ok {
			names = collectBindingTargets(leftNode)
		}

		if len(names) == 0 {
			return trx.WalkChildren(ctx)
		}

		decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(node))
		decl.Names = names

		if typeNode, ok := node.ChildByFieldName("type"); ok {
			decl.Type = typeExpression(typeNode)
		}

		if err := trx.Emit(decl); err != nil {
			return err
		}

		// RHS: walk into decl so call expressions on the right-hand side
		// land in Virtual
		return trx.WalkChildrenInto(ctx, decl)

	case "type_alias_statement":
		// A member-less alias needs only a declaration on its name
		decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(node))

		if leftNode, ok := node.ChildByFieldName("left"); ok {
			decl.Names = aliasNames(leftNode)
		}

		if len(decl.Names) == 0 {
			return trx.WalkChildren(ctx)
		}

		if rightNode, ok := node.ChildByFieldName("right"); ok {
			decl.Type = typeExpression(rightNode)
		}

		return trx.Emit(decl)

	case "for_statement", "for_in_clause":
		if leftNode, ok := node.ChildByFieldName("left"); ok {
			if err := emitBindingDeclaration(trx, leftNode); err != nil {
				return err
			}
		}
		return trx.WalkChildren(ctx)

	case "named_expression":
		if nameNode, ok := node.ChildByFieldName("name"); ok {
			if err := emitBindingDeclaration(trx, nameNode); err != nil {
				return err
			}
		}
		return trx.WalkChildren(ctx)

	case "as_pattern":
		if aliasNode, ok := node.ChildByFieldName("alias"); ok {
			if err := emitBindingDeclaration(trx, aliasNode); err != nil {
				return err
			}
		}
		return trx.WalkChildren(ctx)

	case "call":
		callExpr := ast.NewASTCallExpression(ast.NewASTNodeContainer(node))

		if fnNode, ok := node.ChildByFieldName("function"); ok {
			switch fnNode.Kind {
			case "attribute":
				if object, ok := fnNode.ChildByFieldName("object"); ok && object.Kind == "identifier" {
					callExpr.Namespace = ast.NewASTSymbol(ast.NewASTNodeContainer(object), object.GetTextContent())
				}
				if attr, ok := fnNode.ChildByFieldName("attribute"); ok {
					callExpr.Symbol = ast.NewASTSymbol(ast.NewASTNodeContainer(attr), attr.GetTextContent())
				}
			case "identifier":
				callExpr.Symbol = ast.NewASTSymbol(ast.NewASTNodeContainer(fnNode), fnNode.GetTextContent())
			}
		}

		// A call without an identifiable callee cannot resolve through the
		// LSP; its children still index under the enclosing block
		if callExpr.Symbol == nil {
			return trx.WalkChildren(ctx)
		}

		if err := trx.Emit(callExpr); err != nil {
			return err
		}

		// Handles chained and nested calls
		return trx.WalkChildrenInto(ctx, callExpr)

	default:
		return trx.WalkChildren(ctx)
	}
}

type PythonLanguageSupportDefinition struct{}

func NewPythonLanguageSupportDefinition() *PythonLanguageSupportDefinition {
	return &PythonLanguageSupportDefinition{}
}

var (
	typeshedStdlibRE = regexp.MustCompile(`(?i)/typeshed(-fallback)?/stdlib/`)
	typeshedStubsRE  = regexp.MustCompile(`(?i)/typeshed(-fallback)?/stubs/`)
	sitePackagesRE   = regexp.MustCompile(`(?i)/(site|dist)-packages/`)
	// Interpreter stdlib: Unix keeps it under lib/pythonX.Y; Windows keeps
	// it at <install root>/Lib across the mainstream install layouts
	// (python.org, pymanager, uv, pyenv-win, Store Python, conda)
	pythonLibRE = regexp.MustCompile(`(?i)(/lib/python\d+(\.\d+)?/` +
		`|/(python\d[^/]*|pythoncore-[^/]+|cpython-[^/]+|pyenv-win/versions/[^/]+|pythonsoftwarefoundation\.python[^/]*|[a-z]*conda\d*)/lib/)`)
)

func (plsd *PythonLanguageSupportDefinition) ClassifyImportType(s *common.Source) common.DependencyType {
	p := filepath.ToSlash(filepath.Clean(s.Path))

	if typeshedStdlibRE.MatchString(p) {
		return common.StandardLibraryDependency
	}

	if typeshedStubsRE.MatchString(p) || sitePackagesRE.MatchString(p) {
		return common.PackageDependency
	}

	// Interpreter-installed stdlib sources, after site-packages has been
	// ruled out (site-packages lives under lib/pythonX.Y too)
	if pythonLibRE.MatchString(p) {
		return common.StandardLibraryDependency
	}

	return common.LocalDependency
}

func (plsd *PythonLanguageSupportDefinition) GetLSPServerRequirement() lsp.ServerRequirement {
	// banstructlit:ignore
	return lsp.ServerRequirement{
		Name:           pyrightServerName,
		InstallCommand: pyrightInstallCommand,
		Locate:         pyrightPath,
	}
}

func (plsd *PythonLanguageSupportDefinition) NewLSPServer(ctx context.Context) (*lsp.ServerProcess, error) {
	execPath, err := pyrightPath(ctx)

	if err != nil {
		return nil, err
	}

	// pyright only speaks LSP over stdio when asked; without the flag it
	// hangs at initialize
	cmd := exec.CommandContext(ctx, execPath, "--stdio")
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

func (plsd *PythonLanguageSupportDefinition) Parse(ctx context.Context, src *common.Source, tree *tree_sitter.Tree) (*ast.ASTModule, error) {
	name := strings.TrimSuffix(filepath.Base(src.Path), filepath.Ext(src.Path))

	root := ast.NewASTModule(ast.NewASTNodeContainer(
		parsers.NewParserNode(src, tree.RootNode()),
	), name)

	if err := parsers.WalkTransformTree(ctx, src, tree, root, PythonTransformer); err != nil {
		return nil, err
	}

	return root, nil
}

func (plsd *PythonLanguageSupportDefinition) GetLanguageID() string {
	return "python"
}

func (plsd *PythonLanguageSupportDefinition) GetTreeSitterLanguage() *tree_sitter.Language {
	return tree_sitter.NewLanguage(tree_sitter_python.Language())
}
