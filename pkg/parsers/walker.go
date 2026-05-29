package parsers

import (
	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/common"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// Simple abstraction to not lock the implementation to tree_sitter
type ParserNode struct {
	Source *common.Source
	Kind   string
	raw    *tree_sitter.Node
	Range  tree_sitter.Range // TODO: Replace with module / file / pointer equivalent
}

func (apn ParserNode) GetSource() *common.Source {
	return apn.Source
}

func (apn ParserNode) Debug() string {
	return apn.raw.ToSexp()
}

func (apn ParserNode) GetStart() ast.ASTRealizedPoint {
	return ast.ASTRealizedPoint{
		Line:   int(apn.Range.StartPoint.Row) + 1,
		Column: int(apn.Range.StartPoint.Column) + 1,
	}
}

func (apn ParserNode) IterateChildren(iterator func(ParserNode) (bool, error)) error {
	var e error
	for i := uint(0); i < apn.raw.NamedChildCount(); i++ {
		child := apn.raw.NamedChild(i)
		node := NewParserNode(apn.Source, child)

		shouldContinue, err := iterator(node)

		if err != nil {
			e = err
			break
		}

		if !shouldContinue {
			break
		}
	}

	return e
}

func (apn ParserNode) GetNthChildByKind(kind string, n int) (ParserNode, bool) {
	matches := 0

	for i := uint(0); i < apn.raw.NamedChildCount(); i++ {
		child := apn.raw.NamedChild(i)
		if child != nil && child.Kind() == kind {
			matches++

			if matches > n { // n is always an index so matches = 1 is n = 0 therefore matches > n is what we want
				node := NewParserNode(apn.Source, child)
				return node, true
			}
		}
	}

	return ParserNode{}, false
}

func (apn ParserNode) IterateChildrenByFieldName(fieldName string, iterator func(ParserNode) (bool, error)) error {
	count := uint32(apn.raw.NamedChildCount())
	for i := uint32(0); i < count; i++ {
		if apn.raw.FieldNameForNamedChild(i) == fieldName {
			child := apn.raw.NamedChild(uint(i))
			if child == nil {
				continue
			}
			node := NewParserNode(apn.Source, child)
			shouldContinue, err := iterator(node)
			if err != nil {
				return err
			}
			if !shouldContinue {
				break
			}
		}
	}
	return nil
}

func (apn ParserNode) ChildByFieldName(name string) (ParserNode, bool) {
	child := apn.raw.ChildByFieldName(name)

	if child == nil {
		return ParserNode{}, false
	}

	nn := NewParserNode(apn.Source, child)

	return nn, true
}

func (apn ParserNode) GetTextContent() string {
	return apn.raw.Utf8Text(apn.Source.Buffer)
}

func (apn ParserNode) GetStop() ast.ASTRealizedPoint {
	return ast.ASTRealizedPoint{
		Line:   int(apn.Range.EndPoint.Row) + 1,
		Column: int(apn.Range.EndPoint.Column) + 1,
	}
}

func (apn ParserNode) GetRange() [2]int {
	return [2]int{
		int(apn.Range.StartByte),
		int(apn.Range.EndByte),
	}
}

var _ ast.ASTParserNode = &ParserNode{}

func NewParserNode(src *common.Source, node *tree_sitter.Node) ParserNode {
	return ParserNode{
		Source: src,
		Kind:   node.Kind(),
		raw:    node,
		Range:  node.Range(),
	}
}

type AttachNode func(parent ast.ASTNode, child ast.ASTNode) error

type TransformNode func(ctx *TransformContext, node ParserNode) error

type TransformContext struct {
	cursor *tree_sitter.TreeCursor
	Root   *ast.ASTModule
	Parent ast.ASTNode
	attach AttachNode
	walk   func(parent ast.ASTNode) error
}

func (ctx *TransformContext) Emit(child ast.ASTNode) error {
	return ctx.attach(ctx.Parent, child)
}

func (ctx *TransformContext) WalkChildren() error {
	return ctx.walk(ctx.Parent)
}

func (ctx *TransformContext) WalkChildrenInto(parent ast.ASTNode) error {
	return ctx.walk(parent)
}

func attach(parent ast.ASTNode, child ast.ASTNode) error {
	parent.AppendChild(child)
	return nil
}

func WalkTransformTree(
	src *common.Source,
	tree *tree_sitter.Tree,
	root *ast.ASTModule,
	transform TransformNode,
) error {
	cursor := tree.Walk()
	defer cursor.Close()

	ctx := &TransformContext{
		Root:   root,
		cursor: cursor,
		attach: attach,
	}

	var walk func(parent ast.ASTNode) error

	walk = func(parent ast.ASTNode) error {
		if !cursor.GotoFirstChild() {
			return nil
		}

		defer cursor.GotoParent()

		previousParent := ctx.Parent
		ctx.Parent = parent
		defer func() {
			ctx.Parent = previousParent
		}()

		for {
			node := NewParserNode(src, cursor.Node())

			if err := transform(ctx, node); err != nil {
				return err
			}

			if !cursor.GotoNextSibling() {
				return nil
			}

			ctx.Parent = parent
		}
	}

	ctx.walk = walk

	return walk(root)
}
