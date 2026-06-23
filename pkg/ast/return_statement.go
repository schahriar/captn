package ast

import (
	"fmt"

	"github.com/schahriar/captn/pkg/common"
)

type ASTReturnStatement struct {
	*ASTNodeContainer

	Virtual []ASTNode
}

func (node *ASTReturnStatement) Kind() string {
	return "Return"
}

func (node *ASTReturnStatement) NearestOrSelf(filt func(ASTNode) bool) ASTNode {
	return node.ASTNodeContainer.NearestOrSelf(node, filt)
}

func NewASTReturnStatement(cont *ASTNodeContainer) *ASTReturnStatement {
	node := &ASTReturnStatement{
		ASTNodeContainer: cont.Clone(),
		Virtual:          []ASTNode{},
	}
	return node
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

func (node *ASTReturnStatement) GetHash() common.HashType {
	return GetHash(node)
}

func (node *ASTReturnStatement) DebugPosition() ASTNodeSourcePosition {
	return GetDebugPosition(node)
}

func (node *ASTReturnStatement) MarshalYAML() ([]byte, error) {
	return marshalNodeYAML(node)
}

func (node *ASTReturnStatement) MarshalJSON() ([]byte, error) {
	return marshalNodeJSON(node)
}
