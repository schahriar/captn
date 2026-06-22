package ast

import (
	"context"
	"encoding/json"
	"fmt"

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

func NewASTNodeSourcePosition(position, sourceHash string) ASTNodeSourcePosition {
	return ASTNodeSourcePosition{
		Position:   position,
		SourceHash: sourceHash,
	}
}

func (cont *ASTNodeContainer) GetRawSource() []byte {
	fr := cont.Node.GetPosition()
	src := cont.Node.GetSource()

	nr := fr.GetByteRange()
	return src.Buffer[nr[0]:nr[1]]
}

func (cont *ASTNodeContainer) GetPosition() *common.FileRange {
	return cont.Node.GetPosition()
}

func (cont *ASTNodeContainer) GetHash() uint32 {
	return common.PrimaryHash(cont.GetRawSource())
}

func (cont *ASTNodeContainer) DebugPosition() ASTNodeSourcePosition {
	pos := cont.Node.GetPosition()
	hash := cont.GetHash()

	return NewASTNodeSourcePosition(pos.String(), fmt.Sprintf("%08x\n", hash))
}

func (cont *ASTNodeContainer) MarshalYAML() ([]byte, error) {
	return yaml.Marshal(cont.DebugPosition())
}

func (cont *ASTNodeContainer) MarshalJSON() ([]byte, error) {
	return json.Marshal(cont.DebugPosition())
}

func (cont *ASTNodeContainer) GetStringSource() string {
	return string(cont.GetRawSource())
}

func (cont *ASTNodeContainer) GetFilePath() string {
	return cont.Node.GetSource().Path
}

func (cont *ASTNodeContainer) GetLanguage() string {
	return cont.Node.GetSource().GetLanguage()
}

func NewASTNodeContainer(node ASTParserNode) *ASTNodeContainer {
	return &ASTNodeContainer{
		Node: node,
	}
}

func (node *ASTNodeContainer) ListDependencies(ctx context.Context) (common.ResolvedDependencies, error) {
	return []common.ResolvedDependency{}, nil
}

func (holder *ASTNodeContainer) GetParserNode() ASTParserNode {
	return holder.Node
}

type ASTNode interface {
	GetPosition() *common.FileRange
	GetHash() uint32
	DebugPosition() ASTNodeSourcePosition
	GetRawSource() []byte
	GetParserNode() ASTParserNode
	GetContainer() *ASTNodeContainer
	Children() []ASTNode
	AppendChild(ASTNode)
	String() string
	Kind() string
	Accept(visitor ASTVisitor) interface{}
	GetFilePath() string
	GetLanguage() string
	GetStringSource() string

	ListDependencies(ctx context.Context) (common.ResolvedDependencies, error)
}

type ASTSingularNode interface {
	GetParserNode() ASTParserNode
	Child() ASTNode
	Accept(visitor ASTVisitor) interface{}
}

// Conformance checks
var _ ASTNode = (*ASTModule)(nil)
var _ ASTNode = (*ASTImportStatement)(nil)
var _ ASTNode = (*ASTFuncExpression)(nil)
var _ ASTNode = (*ASTFuncArgument)(nil)
var _ ASTNode = (*ASTBlock)(nil)
var _ ASTNode = (*ASTReturnStatement)(nil)
var _ ASTNode = (*ASTCallExpression)(nil)
var _ ASTNode = (*ASTDeclaration)(nil)
var _ ASTNode = (*ASTSymbol)(nil)

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
