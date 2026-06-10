package ast

import (
	"encoding/json"
	"fmt"
	"hash/crc32"

	"github.com/goccy/go-yaml"
	"github.com/schahriar/captn/pkg/common"
)

type ASTNodeContainer struct {
	Node ASTParserNode
}

type ASTParserNode interface {
	GetSource() *common.Source
	GetPosition() *common.FileRange
}

type ASTNodeSourcePosition struct {
	Position   string
	SourceHash string
}

func (cont *ASTNodeContainer) GetRawSource() []byte {
	nr := cont.Node.GetPosition().GetByteRange()
	return cont.Node.GetSource().Buffer[nr[0]:nr[1]]
}

func (cont *ASTNodeContainer) GetPosition() *common.FileRange {
	return cont.Node.GetPosition()
}

func (cont *ASTNodeContainer) GetSourceHash() uint32 {
	return crc32.ChecksumIEEE(cont.GetRawSource())
}

func (cont *ASTNodeContainer) DebugPosition() ASTNodeSourcePosition {
	pos := cont.Node.GetPosition()
	hash := cont.GetSourceHash()

	return ASTNodeSourcePosition{
		Position:   pos.String(),
		SourceHash: fmt.Sprintf("%08x\n", hash),
	}
}

func (cont *ASTNodeContainer) MarshalYAML() ([]byte, error) {
	return yaml.Marshal(cont.DebugPosition())
}

func (cont *ASTNodeContainer) MarshalJSON() ([]byte, error) {
	return json.Marshal(cont.DebugPosition())
}

func NewASTNodeContainer(node ASTParserNode) *ASTNodeContainer {
	return &ASTNodeContainer{
		Node: node,
	}
}

func (holder *ASTNodeContainer) GetParserNode() ASTParserNode {
	return holder.Node
}

type ASTNode interface {
	GetPosition() *common.FileRange
	GetSourceHash() uint32
	DebugPosition() ASTNodeSourcePosition
	GetRawSource() []byte
	GetParserNode() ASTParserNode
	GetContainer() *ASTNodeContainer
	Children() []ASTNode
	AppendChild(ASTNode)
	String() string
	Kind() string
	Accept(visitor ASTVisitor) interface{}
}

type ASTSingularNode interface {
	GetParserNode() ASTParserNode
	Child() ASTNode
	Accept(visitor ASTVisitor) interface{}
}

// Conformance checks
var _ ASTNode = &ASTModule{}
var _ ASTNode = &ASTImportStatement{}
var _ ASTNode = &ASTFuncExpression{}
var _ ASTNode = &ASTFuncArgument{}
var _ ASTNode = &ASTBlock{}
var _ ASTNode = &ASTReturnStatement{}
var _ ASTNode = &ASTCallExpression{}
var _ ASTNode = &ASTDeclaration{}
var _ ASTNode = &ASTSymbol{}

type ASTVisitor interface {
	VisitModule(*ASTModule) interface{}
	VisitImport(*ASTImportStatement) interface{}
	VisitCallExpression(*ASTCallExpression) interface{}
	VisitFuncExpression(*ASTFuncExpression) interface{}
	VisitFuncArgument(*ASTFuncArgument) interface{}
	VisitDeclaration(*ASTDeclaration) interface{}
	VisitBlock(*ASTBlock) interface{}
	VisitReturn(*ASTReturnStatement) interface{}
	VisitSymbol(*ASTSymbol) interface{}
}
