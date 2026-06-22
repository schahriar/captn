package ast

import (
	"fmt"

	"github.com/schahriar/captn/pkg/knownerr"
)

type ASTSymbol struct {
	*ASTNodeContainer
	Name string
}

func (node *ASTSymbol) Kind() string {
	return "Symbol"
}

func (node *ASTSymbol) NearestOrSelf(filt func(ASTNode) bool) ASTNode {
	return node.ASTNodeContainer.NearestOrSelf(node, filt)
}

func NewASTSymbol(cont *ASTNodeContainer, name string) *ASTSymbol {
	node := &ASTSymbol{
		ASTNodeContainer: cont.Clone(),
		Name:             name,
	}
	return node
}

func (node *ASTSymbol) GetContainer() *ASTNodeContainer {
	return node.ASTNodeContainer
}

func (node *ASTSymbol) Children() []ASTNode {
	return []ASTNode{}
}

func (node *ASTSymbol) AppendChild(n ASTNode) {
	panic(knownerr.DoesNotAcceptChildren(node))
}

func (node *ASTSymbol) String() string {
	return fmt.Sprintf("Symbol:%v", node.Name)
}

func (node *ASTSymbol) Accept(visitor ASTVisitor) interface{} {
	return ASTPanicBoundary(node, func() interface{} {
		return visitor.VisitSymbol(node)
	})
}

// Conformance checks
var _ ASTNode = (*ASTSymbol)(nil)
