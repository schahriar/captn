package ast

import (
	"fmt"
)

type ASTReturnStatement struct {
	*ASTNodeContainer

	Virtual []ASTNode
}

func (node *ASTReturnStatement) Kind() string {
	return "Return"
}

func NewReturnStatement(cont *ASTNodeContainer) *ASTReturnStatement {
	return &ASTReturnStatement{
		ASTNodeContainer: cont,
		Virtual:          []ASTNode{},
	}
}

func (node *ASTReturnStatement) GetContainer() *ASTNodeContainer {
	return node.ASTNodeContainer
}

func (node *ASTReturnStatement) String() string {
	return fmt.Sprintf("ReturnStatement")
}

func (node *ASTReturnStatement) Children() []ASTNode {
	return node.Virtual
}

func (node *ASTReturnStatement) AppendChild(n ASTNode) {
	node.Virtual = append(node.Virtual, n)
}

func (node *ASTReturnStatement) Accept(visitor ASTVisitor) interface{} {
	return ASTPanicBoundary(node, func() interface{} {
		return visitor.VisitReturn(node)
	})
}
