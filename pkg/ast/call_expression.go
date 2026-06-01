package ast

import (
	"fmt"
)

type ASTCallExpression struct {
	*ASTNodeContainer

	Namespace *ASTSymbol
	Symbol    *ASTSymbol

	Arguments []*ASTFuncArgument
	Virtual   []ASTNode
}

func (node *ASTCallExpression) Kind() string {
	return "CallExpression"
}

func NewASTCallExpression(cont *ASTNodeContainer) *ASTCallExpression {
	return &ASTCallExpression{
		ASTNodeContainer: cont,
		Symbol:           nil,
		Namespace:        nil,
		Arguments:        []*ASTFuncArgument{},
		Virtual:          []ASTNode{},
	}
}

func (node *ASTCallExpression) GetContainer() *ASTNodeContainer {
	return node.ASTNodeContainer
}

func (node *ASTCallExpression) String() string {
	if node.Namespace == nil {
		return fmt.Sprintf("CallExpression %v(%v)", node.Symbol, len(node.Arguments))
	}

	return fmt.Sprintf("CallExpression %v.%v(%v)", node.Namespace, node.Symbol, len(node.Arguments))
}

func (node *ASTCallExpression) Children() []ASTNode {
	list := []ASTNode{}

	if node.Namespace != nil {
		list = append(list, node.Namespace)
	}
	if node.Symbol != nil {
		list = append(list, node.Symbol)
	}
	for _, arg := range node.Arguments {
		list = append(list, arg)
	}

	return append(list, node.Virtual...)
}

func (node *ASTCallExpression) AppendChild(n ASTNode) {
	node.Virtual = append(node.Virtual, n)
}

func (node *ASTCallExpression) Accept(visitor ASTVisitor) interface{} {
	return ASTPanicBoundary(node, func() interface{} {
		return visitor.VisitCallExpression(node)
	})
}
