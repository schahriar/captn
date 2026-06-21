package ast

import (
	"github.com/schahriar/captn/pkg/knownerr"
)

type ASTModule struct {
	*ASTNodeContainer

	Name  string
	Block *ASTBlock
}

func (node *ASTModule) GetContainer() *ASTNodeContainer {
	return node.ASTNodeContainer
}

func (node *ASTModule) Children() []ASTNode {
	return []ASTNode{node.Block}
}

func (node *ASTModule) AppendChild(n ASTNode) {
	node.Block.AppendChild(n)
}

func (node *ASTModule) String() string {
	return "Module"
}

func (node *ASTModule) Kind() string {
	return "Module"
}

func (node *ASTModule) Accept(visitor ASTVisitor) interface{} {
	return ASTPanicBoundary(node, func() interface{} {
		return visitor.VisitModule(node)
	})
}

func (node *ASTModule) Debug() interface{} {
	debug := &InspectingVisitor{}
	return node.Accept(debug)
}

func NewASTModule(cont *ASTNodeContainer, name string) *ASTModule {
	return &ASTModule{
		ASTNodeContainer: cont,
		Name:             name,
		Block:            NewASTBlock(cont),
	}
}

// ASTImportStatement - We treat imports as a node to support dynamic imports
type ASTImportStatement struct {
	*ASTNodeContainer

	Namespace *ASTSymbol // Nil is valid for languages that don't namespace imports by default
	Reference *ASTSymbol // Doesn't have to be a file system path, anything LSP considers valid
}

func NewASTImportStatement(cont *ASTNodeContainer) *ASTImportStatement {
	return &ASTImportStatement{
		ASTNodeContainer: cont,
	}
}

func (node *ASTImportStatement) GetContainer() *ASTNodeContainer {
	return node.ASTNodeContainer
}

func (node *ASTImportStatement) Kind() string {
	return "Import"
}

func (node *ASTImportStatement) Children() []ASTNode {
	list := []ASTNode{}
	if node.Namespace != nil {
		list = append(list, node.Namespace)
	}
	if node.Reference != nil {
		list = append(list, node.Reference)
	}
	return list
}

func (node *ASTImportStatement) AppendChild(n ASTNode) {
	panic(knownerr.DoesNotAcceptChildren(node))
}

func (node *ASTImportStatement) String() string {
	return "Module"
}

func (node *ASTImportStatement) Accept(visitor ASTVisitor) interface{} {
	return ASTPanicBoundary(node, func() interface{} {
		return visitor.VisitImport(node)
	})
}

func (node *ASTImportStatement) Debug() interface{} {
	debug := &InspectingVisitor{}
	return node.Accept(debug)
}
