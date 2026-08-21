package parsers

import (
	"context"
	"fmt"
	"runtime/trace"

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

func (apn ParserNode) GetPosition() *common.FileRange {
	return common.NewFileRange(
		apn.Source,
		common.NewFilePosition(
			apn.Source,
			int(apn.Range.StartPoint.Row),
			int(apn.Range.StartPoint.Column),
			int(apn.Range.StartByte),
		),
		common.NewFilePosition(
			apn.Source,
			int(apn.Range.EndPoint.Row),
			int(apn.Range.EndPoint.Column),
			int(apn.Range.EndByte),
		),
	)
}

func (apn ParserNode) IterateChildren(iterator func(ParserNode) (bool, error)) error {
	cursor := apn.raw.Walk()
	defer cursor.Close()

	for ok := cursor.GotoFirstChild(); ok; ok = cursor.GotoNextSibling() {
		child := cursor.Node()

		if !child.IsNamed() {
			continue
		}

		shouldContinue, err := iterator(NewParserNode(apn.Source, child))

		if err != nil {
			return err
		}

		if !shouldContinue {
			return nil
		}
	}

	return nil
}

// Anonymous tokens included
func (apn ParserNode) IterateAllChildren(iterator func(child ParserNode, field string) (bool, error)) error {
	cursor := apn.raw.Walk()
	defer cursor.Close()

	for ok := cursor.GotoFirstChild(); ok; ok = cursor.GotoNextSibling() {
		shouldContinue, err := iterator(NewParserNode(apn.Source, cursor.Node()), cursor.FieldName())

		if err != nil {
			return err
		}

		if !shouldContinue {
			return nil
		}
	}

	return nil
}

func (apn ParserNode) GetNthChildByKind(kind string, n int) (ParserNode, bool) {
	cursor := apn.raw.Walk()
	defer cursor.Close()

	matches := 0

	for ok := cursor.GotoFirstChild(); ok; ok = cursor.GotoNextSibling() {
		child := cursor.Node()

		if !child.IsNamed() || child.Kind() != kind {
			continue
		}

		matches++

		// n is an index, so the (n+1)th match is the one
		if matches > n {
			return NewParserNode(apn.Source, child), true
		}
	}

	var zero ParserNode
	return zero, false
}

func (apn ParserNode) IterateChildrenByFieldName(fieldName string, iterator func(ParserNode) (bool, error)) error {
	cursor := apn.raw.Walk()
	defer cursor.Close()

	for ok := cursor.GotoFirstChild(); ok; ok = cursor.GotoNextSibling() {
		child := cursor.Node()

		if !child.IsNamed() || cursor.FieldName() != fieldName {
			continue
		}

		shouldContinue, err := iterator(NewParserNode(apn.Source, child))

		if err != nil {
			return err
		}

		if !shouldContinue {
			return nil
		}
	}

	return nil
}

func (apn ParserNode) ChildByFieldName(name string) (ParserNode, bool) {
	child := apn.raw.ChildByFieldName(name)

	if child == nil {
		var zero ParserNode
		return zero, false
	}

	nn := NewParserNode(apn.Source, child)

	return nn, true
}

func (apn ParserNode) GetTextContent() string {
	return apn.raw.Utf8Text(apn.Source.Buffer)
}

func (apn ParserNode) ParentKind() (string, bool) {
	parent := apn.raw.Parent()

	if parent == nil {
		return "", false
	}

	return parent.Kind(), true
}

func (apn ParserNode) NextNamedSibling() (ParserNode, bool) {
	next := apn.raw.NextNamedSibling()

	if next == nil {
		var zero ParserNode
		return zero, false
	}

	return NewParserNode(apn.Source, next), true
}

var _ ast.ASTParserNode = (*ParserNode)(nil)

func NewParserNode(src *common.Source, node *tree_sitter.Node) ParserNode {
	return ParserNode{
		Source: src,
		Kind:   node.Kind(),
		raw:    node,
		Range:  node.Range(),
	}
}

type AttachNode func(parent ast.ASTNode, child ast.ASTNode) error

type TransformNode func(ctx context.Context, trx *TransformContext, node ParserNode) error

type TransformContext struct {
	cursor *tree_sitter.TreeCursor
	Root   *ast.ASTModule
	Parent ast.ASTNode
	attach AttachNode
	walk   func(ctx context.Context, parent ast.ASTNode) error
}

func NewTransformContext(root *ast.ASTModule, cursor *tree_sitter.TreeCursor, attach AttachNode) *TransformContext {
	return &TransformContext{
		Root:   root,
		cursor: cursor,
		attach: attach,
	}
}

func (trx *TransformContext) Emit(child ast.ASTNode) error {
	return trx.attach(trx.Parent, child)
}

func (trx *TransformContext) WalkChildren(ctx context.Context) error {
	return trx.walk(ctx, trx.Parent)
}

func (trx *TransformContext) WalkChildrenInto(ctx context.Context, parent ast.ASTNode) error {
	return trx.walk(ctx, parent)
}

func attach(parent ast.ASTNode, child ast.ASTNode) error {
	parent.AppendChild(child)
	return nil
}

func WalkTransformTree(
	ctx context.Context,
	src *common.Source,
	tree *tree_sitter.Tree,
	root *ast.ASTModule,
	transform TransformNode,
) error {
	cursor := tree.Walk()
	defer cursor.Close()

	trx := NewTransformContext(root, cursor, attach)

	var walk func(ctx context.Context, parent ast.ASTNode) error

	walk = func(ctx context.Context, parent ast.ASTNode) error {
		if !cursor.GotoFirstChild() {
			return nil
		}

		defer cursor.GotoParent()

		previousParent := trx.Parent
		trx.Parent = parent
		defer func() {
			trx.Parent = previousParent
		}()

		for {
			node := NewParserNode(src, cursor.Node())

			var err error

			trace.WithRegion(ctx, fmt.Sprintf("transform(%v)", node.Kind), func() {
				err = transform(ctx, trx, node)
			})

			if err != nil {
				return err
			}

			if !cursor.GotoNextSibling() {
				return nil
			}

			trx.Parent = parent
		}
	}

	trx.walk = walk

	ctx, task := trace.NewTask(ctx, "walkTree")
	defer task.End()

	if err := walk(ctx, root); err != nil {
		return err
	}

	ast.AttachParents(root)
	return nil
}
