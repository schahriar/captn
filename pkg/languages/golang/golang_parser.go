package languages_golang

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/lsp"
	"github.com/schahriar/captn/pkg/parsers"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_golang "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

// TODO: Support parsing go embeds as references
// TODO: Support comments

// hasSourceWidth reports whether a node spans any bytes; tree-sitter error
// recovery produces zero-width nodes that must never become AST nodes (the
// interval index rejects empty ranges)
func hasSourceWidth(node parsers.ParserNode) bool {
	return node.Range.StartByte < node.Range.EndByte
}

func collectIdentifiers(node parsers.ParserNode) []*ast.ASTSymbol {
	if node.Kind == "identifier" {
		if !hasSourceWidth(node) {
			return nil
		}
		return []*ast.ASTSymbol{ast.NewASTSymbol(ast.NewASTNodeContainer(node), node.GetTextContent())}
	}
	var syms []*ast.ASTSymbol
	node.IterateChildren(func(child parsers.ParserNode) (bool, error) {
		syms = append(syms, collectIdentifiers(child)...)
		return true, nil
	})
	return syms
}

// typeExpression maps a type onto the identifier a definition resolves to.
// Nameless composites (pointers, slices, maps, function types) fold flat:
// the first named type inside is the head and the rest are its arguments.
func typeExpression(node parsers.ParserNode) *ast.ASTTypeExpression {
	switch node.Kind {
	case "type_identifier":
		if !hasSourceWidth(node) {
			return nil
		}

		return ast.NewASTTypeExpression(ast.NewASTNodeContainer(node), node.GetTextContent())

	case "qualified_type":
		nameNode, ok := node.ChildByFieldName("name")

		if !ok || !hasSourceWidth(nameNode) {
			return nil
		}

		texpr := ast.NewASTTypeExpression(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())

		if pkgNode, ok := node.ChildByFieldName("package"); ok && hasSourceWidth(pkgNode) {
			texpr.Namespace = ast.NewASTSymbol(ast.NewASTNodeContainer(pkgNode), pkgNode.GetTextContent())
		}

		return texpr

	case "generic_type":
		headNode, ok := node.ChildByFieldName("type")

		if !ok {
			return nil
		}

		head := typeExpression(headNode)

		if head == nil {
			return nil
		}

		if argsNode, ok := node.ChildByFieldName("type_arguments"); ok {
			head.Arguments = append(head.Arguments, collectTypes(argsNode)...)
		}

		return head
	}

	types := collectTypes(node)

	if len(types) == 0 {
		return nil
	}

	types[0].Arguments = append(types[0].Arguments, types[1:]...)

	return types[0]
}

func collectTypes(node parsers.ParserNode) []*ast.ASTTypeExpression {
	var types []*ast.ASTTypeExpression

	node.IterateChildren(func(child parsers.ParserNode) (bool, error) {
		switch child.Kind {
		case "type_identifier", "qualified_type", "generic_type":
			if texpr := typeExpression(child); texpr != nil {
				types = append(types, texpr)
			}
		default:
			types = append(types, collectTypes(child)...)
		}

		return true, nil
	})

	return types
}

// Several names can share one declaration (`a, b int`); each name past the
// first gets an argument on its own identifier so a definition resolving
// onto it still finds a node there
func appendParameter(fn *ast.ASTFuncExpression, decl parsers.ParserNode) {
	var texpr *ast.ASTTypeExpression

	if typeNode, ok := decl.ChildByFieldName("type"); ok {
		texpr = typeExpression(typeNode)
	}

	var names []parsers.ParserNode

	decl.IterateChildrenByFieldName("name", func(nameNode parsers.ParserNode) (bool, error) {
		if hasSourceWidth(nameNode) {
			names = append(names, nameNode)
		}
		return true, nil
	})

	if len(names) == 0 {
		fn.Arguments = append(fn.Arguments, ast.NewASTFuncArgument(ast.NewASTNodeContainer(decl), nil, texpr))
		return
	}

	for i, nameNode := range names {
		sym := ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())

		if i == 0 {
			fn.Arguments = append(fn.Arguments, ast.NewASTFuncArgument(ast.NewASTNodeContainer(decl), sym, texpr))
			continue
		}

		fn.Arguments = append(fn.Arguments, ast.NewASTFuncArgument(ast.NewASTNodeContainer(nameNode), sym, nil))
	}
}

func appendParameters(fn *ast.ASTFuncExpression, list parsers.ParserNode) {
	list.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
		switch pn.Kind {
		case "parameter_declaration", "variadic_parameter_declaration":
			appendParameter(fn, pn)
		}
		return true, nil
	})
}

func appendTypeParameters(fn *ast.ASTFuncExpression, decl parsers.ParserNode) {
	list, ok := decl.ChildByFieldName("type_parameters")

	if !ok {
		return
	}

	list.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
		if pn.Kind == "type_parameter_declaration" {
			appendParameter(fn, pn)
		}
		return true, nil
	})
}

// A multi-value result is a parameter list and lands in Arguments instead
func returnType(decl parsers.ParserNode) *ast.ASTTypeExpression {
	resultNode, ok := decl.ChildByFieldName("result")

	if !ok || resultNode.Kind == "parameter_list" {
		return nil
	}

	return typeExpression(resultNode)
}

func emitNamedType(trx *parsers.TransformContext, container parsers.ParserNode, spec parsers.ParserNode) error {
	fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(container))

	if nameNode, ok := spec.ChildByFieldName("name"); ok && hasSourceWidth(nameNode) {
		fn.Name = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
	}

	appendTypeParameters(fn, spec)

	typeNode, ok := spec.ChildByFieldName("type")

	if !ok {
		return trx.Emit(fn)
	}

	fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(typeNode))

	switch typeNode.Kind {
	case "struct_type":
		appendStructFields(fn, typeNode)
	case "interface_type":
		appendInterfaceMembers(fn, typeNode)
	default:
		if texpr := typeExpression(typeNode); texpr != nil {
			fn.AppendChild(texpr)
		}
	}

	return trx.Emit(fn)
}

func appendStructFields(fn *ast.ASTFuncExpression, structNode parsers.ParserNode) {
	list, ok := structNode.GetNthChildByKind("field_declaration_list", 0)

	if !ok {
		return
	}

	list.IterateChildren(func(field parsers.ParserNode) (bool, error) {
		if field.Kind != "field_declaration" {
			return true, nil
		}

		decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(field))

		field.IterateChildrenByFieldName("name", func(nameNode parsers.ParserNode) (bool, error) {
			if hasSourceWidth(nameNode) {
				decl.Names = append(decl.Names, ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent()))
			}
			return true, nil
		})

		if typeNode, ok := field.ChildByFieldName("type"); ok {
			decl.Type = typeExpression(typeNode)
		}

		if len(decl.Names) > 0 || decl.Type != nil {
			fn.AppendChild(decl)
		}

		return true, nil
	})
}

func appendInterfaceMembers(fn *ast.ASTFuncExpression, ifaceNode parsers.ParserNode) {
	ifaceNode.IterateChildren(func(member parsers.ParserNode) (bool, error) {
		switch member.Kind {
		case "method_elem":
			method := ast.NewASTFuncExpression(ast.NewASTNodeContainer(member))

			if nameNode, ok := member.ChildByFieldName("name"); ok && hasSourceWidth(nameNode) {
				method.Name = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
			}

			if params, ok := member.ChildByFieldName("parameters"); ok {
				appendParameters(method, params)
			}

			if result, ok := member.ChildByFieldName("result"); ok && result.Kind == "parameter_list" {
				appendParameters(method, result)
			}

			method.ReturnType = returnType(member)
			fn.AppendChild(method)

		case "type_elem":
			if texpr := typeExpression(member); texpr != nil {
				fn.AppendChild(texpr)
			}
		}

		return true, nil
	})
}

func GolangTransformer(ctx context.Context, trx *parsers.TransformContext, node parsers.ParserNode) error {
	switch node.Kind {
	case "package_clause":
		child, ok := node.GetNthChildByKind("package_identifier", 0)

		if ok {
			packageName := child.GetTextContent()
			trx.Root.Name = packageName
		}

		return nil

	case "import_spec":
		impExpr := ast.NewASTImportStatement(ast.NewASTNodeContainer(node))
		if nameNode, ok := node.ChildByFieldName("name"); ok {
			name := nameNode.GetTextContent()
			impExpr.Namespace = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), name)
		}

		if pathNode, ok := node.ChildByFieldName("path"); ok {
			if subNode, ok := pathNode.GetNthChildByKind("interpreted_string_literal_content", 0); ok {
				path := subNode.GetTextContent()
				impExpr.Reference = ast.NewASTSymbol(ast.NewASTNodeContainer(pathNode), path)
			}
		}

		if impExpr.Reference != nil {
			if err := trx.Emit(impExpr); err != nil {
				return err
			}
		}

		return nil

	case "function_declaration", "method_declaration":
		fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))

		if nameNode, ok := node.ChildByFieldName("name"); ok && hasSourceWidth(nameNode) {
			name := nameNode.GetTextContent()
			fn.Name = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), name)
		}

		appendTypeParameters(fn, node)
		fn.ReturnType = returnType(node)

		if err := trx.Emit(fn); err != nil {
			return err
		}

		// Functions auto-assign a block but we want to narrow down the block for the language itself
		if blockNode, ok := node.GetNthChildByKind("block", 0); ok {
			fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(blockNode))
		}

		return trx.WalkChildrenInto(ctx, fn)

	case "type_declaration":
		// A single spec is ranged on the whole declaration so a snippet starting
		// at the keyword roots at the type rather than the module
		var specs []parsers.ParserNode

		node.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
			if pn.Kind == "type_spec" || pn.Kind == "type_alias" {
				specs = append(specs, pn)
			}
			return true, nil
		})

		if len(specs) == 1 {
			return emitNamedType(trx, node, specs[0])
		}

		return trx.WalkChildren(ctx)

	case "type_spec", "type_alias":
		return emitNamedType(trx, node, node)

	case "func_literal":
		fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))
		fn.ReturnType = returnType(node)

		if blockNode, ok := node.GetNthChildByKind("block", 0); ok {
			fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(blockNode))
		}

		if err := trx.Emit(fn); err != nil {
			return err
		}

		return trx.WalkChildrenInto(ctx, fn)

	case "parameter_list":
		// A list under a function type or an interface method belongs to a
		// type, which typeExpression already folded
		fn, ok := trx.Parent.(*ast.ASTFuncExpression)

		if !ok {
			return nil
		}

		switch kind, _ := node.ParentKind(); kind {
		case "function_declaration", "method_declaration", "func_literal":
			appendParameters(fn, node)
		}

		return nil

	case "short_var_decl":
		decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(node))

		// LHS: tree-sitter-go uses repeating "left" field names, one per identifier
		node.IterateChildrenByFieldName("left", func(pn parsers.ParserNode) (bool, error) {
			if pn.Kind == "identifier" {
				decl.Names = append(decl.Names, ast.NewASTSymbol(ast.NewASTNodeContainer(pn), pn.GetTextContent()))
			}
			return true, nil
		})

		if err := trx.Emit(decl); err != nil {
			return err
		}
		// RHS: walk into decl so call expressions on the right-hand side land in Virtual
		return trx.WalkChildrenInto(ctx, decl)

	case "var_spec":
		decl := ast.NewASTDeclaration(ast.NewASTNodeContainer(node))

		node.IterateChildrenByFieldName("name", func(nameNode parsers.ParserNode) (bool, error) {
			if hasSourceWidth(nameNode) {
				decl.Names = append(decl.Names, ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent()))
			}
			return true, nil
		})

		if typeNode, ok := node.ChildByFieldName("type"); ok {
			decl.Type = typeExpression(typeNode)
		}

		if err := trx.Emit(decl); err != nil {
			return err
		}
		return trx.WalkChildrenInto(ctx, decl)

	case "call_expression":
		callExpr := ast.NewASTCallExpression(ast.NewASTNodeContainer(node))

		if fnNode, ok := node.ChildByFieldName("function"); ok {
			switch fnNode.Kind {
			case "selector_expression":
				// Skips over chained calls (e.g. x.Y().Z()), chained calls are handled below at `ctx.WalkChildrenInto(callExpr)`
				if operand, ok := fnNode.ChildByFieldName("operand"); ok {
					if operand.Kind == "identifier" {
						callExpr.Namespace = ast.NewASTSymbol(ast.NewASTNodeContainer(operand), operand.GetTextContent())
					}
				}
				if field, ok := fnNode.ChildByFieldName("field"); ok && hasSourceWidth(field) {
					callExpr.Symbol = ast.NewASTSymbol(ast.NewASTNodeContainer(field), field.GetTextContent())
				}
			case "identifier":
				if hasSourceWidth(fnNode) {
					callExpr.Symbol = ast.NewASTSymbol(ast.NewASTNodeContainer(fnNode), fnNode.GetTextContent())
				}
			}
		}

		// No identifiable callee (e.g. a func literal); its children still
		// index under the enclosing block
		if callExpr.Symbol == nil {
			return trx.WalkChildren(ctx)
		}

		if argList, ok := node.GetNthChildByKind("argument_list", 0); ok {
			argList.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
				syms := collectIdentifiers(pn)
				if len(syms) == 0 {
					callExpr.Arguments = append(callExpr.Arguments, ast.NewASTFuncArgument(ast.NewASTNodeContainer(pn), nil, nil))
				} else {
					for _, sym := range syms {
						callExpr.Arguments = append(callExpr.Arguments, ast.NewASTFuncArgument(sym.GetContainer(), sym, nil))
					}
				}
				return true, nil
			})
		}

		if err := trx.Emit(callExpr); err != nil {
			return err
		}
		// Handles chained operands
		return trx.WalkChildrenInto(ctx, callExpr)

	case "return_statement":
		ret := ast.NewASTReturnStatement(ast.NewASTNodeContainer(node))

		// Capture plain identifier return values as symbol references
		node.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
			if pn.Kind == "identifier" {
				ret.AppendChild(ast.NewASTSymbol(ast.NewASTNodeContainer(pn), pn.GetTextContent()))
			}
			return true, nil
		})

		if err := trx.Emit(ret); err != nil {
			return err
		}

		return trx.WalkChildrenInto(ctx, ret)

	case "comment":
		// Comments are a bit tough, they both need to be block scoped and also
		// associated to the nearest node
		return nil

	default:
		return trx.WalkChildren(ctx)
	}
}

type GolangLanguageSupportDefinition struct{}

func NewGolangLanguageSupportDefinition() *GolangLanguageSupportDefinition {
	return &GolangLanguageSupportDefinition{}
}

var (
	toolchainStdlibRE = regexp.MustCompile(`/pkg/mod/golang\.org/toolchain@[^/]+/src/`)
	gomodcacheRE      = regexp.MustCompile(`/pkg/mod/`)
	vendorRE          = regexp.MustCompile(`(^|/)vendor(/|$)`)
)

// Stdlib sources sit under GOROOT except for module toolchains, so the
// toolchain is asked rather than guessed at. GOTOOLCHAIN=local keeps the
// question from downloading a newer toolchain go.mod may ask for.
var goRootSrc = sync.OnceValue(func() string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "env", "GOROOT")
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local")

	out, err := cmd.Output()

	if err != nil {
		return ""
	}

	root := strings.TrimSpace(string(out))

	if root == "" {
		return ""
	}

	return filepath.ToSlash(filepath.Clean(root)) + "/src/"
})

func (glsd *GolangLanguageSupportDefinition) ClassifyImportType(s *common.Source) common.DependencyType {
	p := s.Path

	// Workspace-relative paths must be anchored back before comparing to GOROOT
	if !filepath.IsAbs(p) && s.Workspace != "" {
		p = filepath.Join(s.Workspace, p)
	}

	p = filepath.ToSlash(filepath.Clean(p))

	if toolchainStdlibRE.MatchString(p) {
		return common.StandardLibraryDependency
	}

	if src := goRootSrc(); src != "" && strings.HasPrefix(p, src) {
		return common.StandardLibraryDependency
	}

	if vendorRE.MatchString(p) || gomodcacheRE.MatchString(p) {
		return common.PackageDependency
	}

	return common.LocalDependency
}

func (glsd *GolangLanguageSupportDefinition) GetLSPServerRequirement() lsp.ServerRequirement {
	// banstructlit:ignore
	return lsp.ServerRequirement{
		Name:           goplsName,
		InstallCommand: goplsInstallCommand,
		Locate:         goplsPath,
	}
}

func (glsd *GolangLanguageSupportDefinition) NewLSPServer(ctx context.Context) (*lsp.ServerProcess, error) {
	execPath, err := goplsPath(ctx)

	if err != nil {
		return nil, err
	}

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

func (glsd *GolangLanguageSupportDefinition) Parse(ctx context.Context, src *common.Source, tree *tree_sitter.Tree) (*ast.ASTModule, error) {
	root := ast.NewASTModule(ast.NewASTNodeContainer(
		parsers.NewParserNode(src, tree.RootNode()),
	), src.Path)

	if err := parsers.WalkTransformTree(ctx, src, tree, root, GolangTransformer); err != nil {
		return nil, err
	}

	return root, nil
}

func (glsd *GolangLanguageSupportDefinition) GetLanguageID() string {
	return "go"
}

func (glsd *GolangLanguageSupportDefinition) GetTreeSitterLanguage() *tree_sitter.Language {
	return tree_sitter.NewLanguage(tree_sitter_golang.Language())
}
