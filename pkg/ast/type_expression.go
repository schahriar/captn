package ast

import (
	"fmt"

	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/knownerr"
)

// ASTTypeExpression - A type as it is used, not as it is declared. Ranged on
// the identifier a definition resolves to; the name is carried inline because
// a child symbol would share a leaf type's exact byte range.
type ASTTypeExpression struct {
	*ASTNodeContainer

	Name      string
	Namespace *ASTSymbol
	Arguments []*ASTTypeExpression
}

func (node *ASTTypeExpression) Kind() string {
	return "TypeExpression"
}

func (node *ASTTypeExpression) NearestOrSelf(filt func(ASTNode) bool) ASTNode {
	return node.ASTNodeContainer.NearestOrSelf(node, filt)
}

func NewASTTypeExpression(cont *ASTNodeContainer, name string) *ASTTypeExpression {
	node := &ASTTypeExpression{
		ASTNodeContainer: cont.Clone(),
		Name:             name,
		Arguments:        []*ASTTypeExpression{},
	}
	return node
}

func (node *ASTTypeExpression) GetContainer() *ASTNodeContainer {
	return node.ASTNodeContainer
}

func (node *ASTTypeExpression) String() string {
	if node.Namespace == nil {
		return fmt.Sprintf("TypeExpression:%v", node.Name)
	}

	return fmt.Sprintf("TypeExpression:%v.%v", node.Namespace.Name, node.Name)
}

func (node *ASTTypeExpression) Children() []ASTNode {
	list := []ASTNode{}

	if node.Namespace != nil {
		list = append(list, node.Namespace)
	}

	for _, arg := range node.Arguments {
		list = append(list, arg)
	}

	return list
}

func (node *ASTTypeExpression) AppendChild(n ASTNode) {
	panic(knownerr.DoesNotAcceptChildren(node))
}

func (node *ASTTypeExpression) Accept(visitor ASTVisitor) interface{} {
	return ASTPanicBoundary(node, func() interface{} {
		return visitor.VisitTypeExpression(node)
	})
}

func (node *ASTTypeExpression) GetHash() common.HashType {
	return GetHash(node)
}

func (node *ASTTypeExpression) DebugPosition() ASTNodeSourcePosition {
	return GetDebugPosition(node)
}

func (node *ASTTypeExpression) MarshalYAML() ([]byte, error) {
	return marshalNodeYAML(node)
}

func (node *ASTTypeExpression) MarshalJSON() ([]byte, error) {
	return marshalNodeJSON(node)
}
