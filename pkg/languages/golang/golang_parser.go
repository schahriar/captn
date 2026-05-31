package languages_golang

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/lsp"
	"github.com/schahriar/captn/pkg/parsers"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_golang "github.com/tree-sitter/tree-sitter-go/bindings/go"
)

// TODO: Support parsing go embeds as references
// TODO: Support comments

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

	case "function_declaration":
		fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))

		if nameNode, ok := node.ChildByFieldName("name"); ok {
			name := nameNode.GetTextContent()
			fn.Name = &name
		}

		if resultNode, ok := node.ChildByFieldName("result"); ok {
			// parameter_list results (multi-return) are captured via WalkChildrenInto below
			if resultNode.Kind != "parameter_list" {
				fn.ReturnType = ast.NewASTSymbol(ast.NewASTNodeContainer(resultNode), resultNode.GetTextContent())
			}
		}

		if err := trx.Emit(fn); err != nil {
			return err
		}

		// Functions auto-assign a block but we want to narrow down the block for the language itself
		if blockNode, ok := node.GetNthChildByKind("block", 0); ok {
			fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(blockNode))
		}

		return trx.WalkChildrenInto(ctx, fn)

	case "func_literal":
		fn := ast.NewASTFuncExpression(ast.NewASTNodeContainer(node))

		if resultNode, ok := node.ChildByFieldName("result"); ok {
			if resultNode.Kind != "parameter_list" {
				fn.ReturnType = ast.NewASTSymbol(ast.NewASTNodeContainer(resultNode), resultNode.GetTextContent())
			}
		}

		if blockNode, ok := node.GetNthChildByKind("block", 0); ok {
			fn.Block = ast.NewASTBlock(ast.NewASTNodeContainer(blockNode))
		}

		if err := trx.Emit(fn); err != nil {
			return err
		}

		return trx.WalkChildrenInto(ctx, fn)

	case "parameter_list":
		if fn, ok := trx.Parent.(*ast.ASTFuncExpression); ok {
			node.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
				if pn.Kind != "parameter_declaration" {
					return true, nil // Continue
				}

				var idSym *ast.ASTSymbol = nil
				var typeSym *ast.ASTSymbol = nil

				if nameNode, ok := pn.ChildByFieldName("name"); ok {
					idSym = ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent())
				}

				if typeNode, ok := pn.ChildByFieldName("type"); ok {
					typeSym = ast.NewASTSymbol(ast.NewASTNodeContainer(typeNode), typeNode.GetTextContent())
				}

				fn.Arguments = append(fn.Arguments, ast.NewASTFuncArgument(ast.NewASTNodeContainer(node), idSym, typeSym))

				return true, nil
			})
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

		if nameNode, ok := node.ChildByFieldName("name"); ok {
			decl.Names = append(decl.Names, ast.NewASTSymbol(ast.NewASTNodeContainer(nameNode), nameNode.GetTextContent()))
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
				if field, ok := fnNode.ChildByFieldName("field"); ok {
					callExpr.Symbol = ast.NewASTSymbol(ast.NewASTNodeContainer(field), field.GetTextContent())
				}
			case "identifier":
				callExpr.Symbol = ast.NewASTSymbol(ast.NewASTNodeContainer(fnNode), fnNode.GetTextContent())
			}
		}

		if argList, ok := node.GetNthChildByKind("argument_list", 0); ok {
			argList.IterateChildren(func(pn parsers.ParserNode) (bool, error) {
				var sym *ast.ASTSymbol
				switch pn.Kind {
				case "identifier":
					sym = ast.NewASTSymbol(ast.NewASTNodeContainer(pn), pn.GetTextContent())
				case "unary_expression":
					// e.g. &X - extract the operand identifier
					if operand, ok := pn.GetNthChildByKind("identifier", 0); ok {
						sym = ast.NewASTSymbol(ast.NewASTNodeContainer(operand), operand.GetTextContent())
					}
				case "selector_expression":
					// e.g. src.Buffer - the operand variable is the reference
					if operand, ok := pn.ChildByFieldName("operand"); ok {
						if operand.Kind == "identifier" {
							sym = ast.NewASTSymbol(ast.NewASTNodeContainer(operand), operand.GetTextContent())
						}
					}
				}
				callExpr.Arguments = append(callExpr.Arguments, ast.NewASTFuncArgument(ast.NewASTNodeContainer(pn), sym, nil))
				return true, nil
			})
		}

		if err := trx.Emit(callExpr); err != nil {
			return err
		}
		// Handles chained operands
		return trx.WalkChildrenInto(ctx, callExpr)

	case "return_statement":
		ret := ast.NewReturnStatement(ast.NewASTNodeContainer(node))

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
		// fmt.Println("Skip", node.Kind)
		return trx.WalkChildren(ctx)
	}
}

type GolangLanguageSupportDefinition struct{}

func (glsd *GolangLanguageSupportDefinition) NewLSPServer(ctx context.Context) (*lsp.ServerProcess, error) {
	gpcmd := exec.Command("go", "env", "GOPATH")
	gopathraw, err := gpcmd.Output()

	if err != nil {
		return nil, err
	}

	gopath := strings.TrimSpace(string(gopathraw))
	execPath := filepath.Join(string(gopath), "./bin/gopls")

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

	return &lsp.ServerProcess{
		Reader: stdout,
		Writer: stdin,
		Wait:   cmd.Wait,
		Kill: func() error {
			return cmd.Process.Kill()
		},
	}, nil
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

func (glsd *GolangLanguageSupportDefinition) GetTreeSitterLanguage() *tree_sitter.Language {
	return tree_sitter.NewLanguage(tree_sitter_golang.Language())
}
