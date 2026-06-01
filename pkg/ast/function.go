package ast

import (
	"fmt"

	"github.com/schahriar/captn/pkg/knownerr"
)

// ASTFuncArgument is shared between call expressions and function arguments
// This will become a semantically bloated type but it's a conscious choice to fit
// the many variations that can act as inputs to a function as a single node
type ASTFuncArgument struct {
	*ASTNodeContainer
	// It's possible for both ID and Symbol to be nil in cases where arguments are read by index
	Identifier *ASTSymbol
	Type       *ASTSymbol
}

func NewASTFuncArgument(cont *ASTNodeContainer, nid *ASTSymbol, nt *ASTSymbol) *ASTFuncArgument {
	return &ASTFuncArgument{
		ASTNodeContainer: cont,
		Identifier:       nid,
		Type:             nt,
	}
}

func (node *ASTFuncArgument) GetContainer() *ASTNodeContainer {
	return node.ASTNodeContainer
}

func (node *ASTFuncArgument) String() string {
	return fmt.Sprintf("FuncArgument(%s: %s)", node.Identifier, node.Type)
}

func (node *ASTFuncArgument) Children() []ASTNode {
	list := []ASTNode{}
	if node.Identifier != nil {
		list = append(list, node.Identifier)
	}
	if node.Type != nil {
		list = append(list, node.Type)
	}
	return list
}

func (node *ASTFuncArgument) AppendChild(n ASTNode) {
	panic(knownerr.DoesNotAcceptChildren(node))
}

func (node *ASTFuncArgument) Accept(visitor ASTVisitor) interface{} {
	return ASTPanicBoundary(node, func() interface{} {
		return visitor.VisitFuncArgument(node)
	})
}

type ASTFuncExpression struct {
	*ASTNodeContainer
	// Assigned as a pointer to force support for null type (anonymous / unnamed functions)
	Name       *ASTSymbol
	Arguments  []*ASTFuncArgument
	ReturnType *ASTSymbol
	Block      *ASTBlock
}

func NewASTFuncExpression(cont *ASTNodeContainer) *ASTFuncExpression {
	// Default is void function with an empty block
	return &ASTFuncExpression{
		ASTNodeContainer: cont,
		Arguments:        []*ASTFuncArgument{},
		ReturnType:       nil,
		Block:            NewASTBlock(cont),
	}
}

func (node *ASTFuncExpression) GetContainer() *ASTNodeContainer {
	return node.ASTNodeContainer
}

func (node *ASTFuncExpression) String() string {
	return fmt.Sprintf("FuncExpression(%v): %v", len(node.Arguments), node.ReturnType)
}

func (node *ASTFuncExpression) Children() []ASTNode {
	list := []ASTNode{}

	if node.Name != nil {
		list = append(list, node.Name)
	}
	for _, arg := range node.Arguments {
		list = append(list, arg)
	}
	if node.ReturnType != nil {
		list = append(list, node.ReturnType)
	}
	list = append(list, node.Block)

	return list
}

func (node *ASTFuncExpression) AppendChild(n ASTNode) {
	node.Block.AppendChild(n)
}

func (node *ASTFuncExpression) Accept(visitor ASTVisitor) interface{} {
	return ASTPanicBoundary(node, func() interface{} {
		return visitor.VisitFuncExpression(node)
	})
}
