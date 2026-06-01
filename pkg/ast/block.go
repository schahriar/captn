package ast

import (
	"fmt"
)

type ASTBlock struct {
	*ASTNodeContainer
	Virtual []ASTNode
}

func (node *ASTBlock) Kind() string {
	return "Block"
}

func (node *ASTBlock) GetContainer() *ASTNodeContainer {
	return node.ASTNodeContainer
}

func (node *ASTBlock) String() string {
	return fmt.Sprintf("Block(virtuals=%v)", len(node.Virtual))
}

func (node *ASTBlock) Children() []ASTNode {
	return node.Virtual
}

func (node *ASTBlock) AppendChild(n ASTNode) {
	node.Virtual = append(node.Virtual, n)
}

func NewASTBlock(cont *ASTNodeContainer) *ASTBlock {
	return &ASTBlock{
		ASTNodeContainer: cont,
		Virtual:          []ASTNode{},
	}
}

func (node *ASTBlock) Accept(visitor ASTVisitor) interface{} {
	return ASTPanicBoundary(node, func() interface{} {
		return visitor.VisitBlock(node)
	})
}
