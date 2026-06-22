package ast

import (
	"fmt"
)

// ASTDeclaration - Any declaration that's not a function declaration
type ASTDeclaration struct {
	*ASTNodeContainer

	// Multi-name allows for destruction or Go / python multi-value returns
	Names   []*ASTSymbol
	Virtual []ASTNode // RHS expressions
}

func (node *ASTDeclaration) Kind() string {
	return "Declaration"
}

func (node *ASTDeclaration) NearestOrSelf(filt func(ASTNode) bool) ASTNode {
	return node.ASTNodeContainer.NearestOrSelf(node, filt)
}

func NewASTDeclaration(cont *ASTNodeContainer) *ASTDeclaration {
	node := &ASTDeclaration{
		ASTNodeContainer: cont.Clone(),
		Names:            []*ASTSymbol{},
		Virtual:          []ASTNode{},
	}
	return node
}

func (node *ASTDeclaration) GetContainer() *ASTNodeContainer {
	return node.ASTNodeContainer
}

func (node *ASTDeclaration) String() string {
	return fmt.Sprintf("Declaration(%v)", len(node.Names))
}

func (node *ASTDeclaration) Children() []ASTNode {
	list := []ASTNode{}
	for _, n := range node.Names {
		list = append(list, n)
	}
	return append(list, node.Virtual...)
}

func (node *ASTDeclaration) AppendChild(n ASTNode) {
	node.Virtual = append(node.Virtual, n)
}

func (node *ASTDeclaration) Accept(visitor ASTVisitor) interface{} {
	return ASTPanicBoundary(node, func() interface{} {
		return visitor.VisitDeclaration(node)
	})
}
