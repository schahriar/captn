package ast

import (
	"fmt"

	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/knownerr"
)

// ASTFuncArgument is shared between call expressions and function arguments
// This will become a semantically bloated type but it's a conscious choice to fit
// the many variations that can act as inputs to a function as a single node
type ASTFuncArgument struct {
	*ASTNodeContainer
	// It's possible for both ID and Symbol to be nil in cases where arguments are read by index
	Identifier *ASTSymbol
	Type       *ASTTypeExpression
}

func (node *ASTFuncArgument) Kind() string {
	return "FuncArgument"
}

func (node *ASTFuncArgument) NearestOrSelf(filt func(ASTNode) bool) ASTNode {
	return node.ASTNodeContainer.NearestOrSelf(node, filt)
}

func NewASTFuncArgument(cont *ASTNodeContainer, nid *ASTSymbol, nt *ASTTypeExpression) *ASTFuncArgument {
	node := &ASTFuncArgument{
		ASTNodeContainer: cont.Clone(),
		Identifier:       nid,
		Type:             nt,
	}
	return node
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

func (node *ASTFuncArgument) GetHash() common.HashType {
	return GetHash(node)
}

func (node *ASTFuncArgument) DebugPosition() ASTNodeSourcePosition {
	return GetDebugPosition(node)
}

func (node *ASTFuncArgument) MarshalYAML() ([]byte, error) {
	return marshalNodeYAML(node)
}

func (node *ASTFuncArgument) MarshalJSON() ([]byte, error) {
	return marshalNodeJSON(node)
}

type ASTFuncExpression struct {
	*ASTNodeContainer
	// Assigned as a pointer to force support for null type (anonymous / unnamed functions)
	Name       *ASTSymbol
	Arguments  []*ASTFuncArgument
	ReturnType *ASTTypeExpression
	Block      *ASTBlock
}

func (node *ASTFuncExpression) Kind() string {
	return "FuncExpression"
}

func (node *ASTFuncExpression) NearestOrSelf(filt func(ASTNode) bool) ASTNode {
	return node.ASTNodeContainer.NearestOrSelf(node, filt)
}

func NewASTFuncExpression(cont *ASTNodeContainer) *ASTFuncExpression {
	// Default is void function with an empty block
	node := &ASTFuncExpression{
		ASTNodeContainer: cont.Clone(),
		Arguments:        []*ASTFuncArgument{},
		ReturnType:       nil,
		Block:            NewASTBlock(cont),
	}
	return node
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

func (node *ASTFuncExpression) GetHash() common.HashType {
	return GetHash(node)
}

func (node *ASTFuncExpression) DebugPosition() ASTNodeSourcePosition {
	return GetDebugPosition(node)
}

func (node *ASTFuncExpression) MarshalYAML() ([]byte, error) {
	return marshalNodeYAML(node)
}

func (node *ASTFuncExpression) MarshalJSON() ([]byte, error) {
	return marshalNodeJSON(node)
}
