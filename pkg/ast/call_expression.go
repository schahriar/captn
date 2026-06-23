package ast

import (
	"fmt"

	"github.com/schahriar/captn/pkg/common"
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

func (node *ASTCallExpression) NearestOrSelf(filt func(ASTNode) bool) ASTNode {
	return node.ASTNodeContainer.NearestOrSelf(node, filt)
}

func NewASTCallExpression(cont *ASTNodeContainer) *ASTCallExpression {
	node := &ASTCallExpression{
		ASTNodeContainer: cont.Clone(),
		Symbol:           nil,
		Namespace:        nil,
		Arguments:        []*ASTFuncArgument{},
		Virtual:          []ASTNode{},
	}
	return node
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

func (node *ASTCallExpression) GetHash() common.HashType {
	return GetHash(node)
}

func (node *ASTCallExpression) DebugPosition() ASTNodeSourcePosition {
	return GetDebugPosition(node)
}

func (node *ASTCallExpression) MarshalYAML() ([]byte, error) {
	return marshalNodeYAML(node)
}

func (node *ASTCallExpression) MarshalJSON() ([]byte, error) {
	return marshalNodeJSON(node)
}
