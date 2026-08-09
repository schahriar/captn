package ast

import (
	"encoding/json"

	"github.com/goccy/go-yaml"
	"github.com/schahriar/captn/pkg/common"
)

type ASTNodeContainer struct {
	Node   ASTParserNode
	parent ASTNode
}

type ASTParserNode interface {
	GetSource() *common.Source
	GetPosition() *common.FileRange
}

type ASTNodeSourcePosition struct {
	Position   string
	SourceHash string
}

// GetHash derives cache identity from workspace-relative paths so the same
// node hashes identically across checkout locations and operating systems
func GetHash(node ASTNode) common.HashType {
	pos := node.GetPosition()
	hash := common.HashMany(
		[]byte(pos.Source.RelativePath()),
		node.GetRawSource(),
		[]byte(pos.RelativeString()),
		[]byte(node.Kind()),
	)

	if node.GetParent() == nil {
		return hash
	}

	hash = hash.Add(GetHash(node.GetParent()))
	return hash
}

func GetDebugPosition(node ASTNode) ASTNodeSourcePosition {
	pos := node.GetPosition()
	hash := GetHash(node)

	return NewASTNodeSourcePosition(pos.String(), hash.String())
}

func marshalNodeYAML(node ASTNode) ([]byte, error) {
	return yaml.Marshal(GetDebugPosition(node))
}

func marshalNodeJSON(node ASTNode) ([]byte, error) {
	return json.Marshal(GetDebugPosition(node))
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

func (cont *ASTNodeContainer) GetFileRange() *common.FileRange {
	return cont.Node.GetPosition()
}

func (cont *ASTNodeContainer) DebugPosition() ASTNodeSourcePosition {
	pos := cont.Node.GetPosition()
	hash := common.HashMany(
		[]byte(pos.Source.RelativePath()),
		cont.GetRawSource(),
		[]byte(pos.RelativeString()),
	)
	if cont.parent != nil {
		hash = hash.Add(GetHash(cont.parent))
	}

	return NewASTNodeSourcePosition(pos.String(), hash.String())
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

func (cont *ASTNodeContainer) Clone() *ASTNodeContainer {
	return NewASTNodeContainer(cont.Node)
}

func (holder *ASTNodeContainer) GetParserNode() ASTParserNode {
	return holder.Node
}

func (holder *ASTNodeContainer) GetParent() ASTNode {
	return holder.parent
}

func (holder *ASTNodeContainer) SetParent(parent ASTNode) {
	holder.parent = parent
}

func (holder *ASTNodeContainer) Nearest(filt func(ASTNode) bool) ASTNode {
	for node := holder.parent; node != nil; node = node.GetParent() {
		if filt(node) {
			return node
		}
	}

	return nil
}

func (holder *ASTNodeContainer) NearestOrSelf(self ASTNode, filt func(ASTNode) bool) ASTNode {
	if filt(self) {
		return self
	}

	return holder.Nearest(filt)
}

func AttachParents(root ASTNode) {
	attachParents(root, nil)
}

func attachParents(node ASTNode, parent ASTNode) {
	if node == nil {
		return
	}

	node.SetParent(parent)

	for _, child := range node.Children() {
		attachParents(child, node)
	}
}

type ASTNode interface {
	GetPosition() *common.FileRange
	GetHash() common.HashType
	DebugPosition() ASTNodeSourcePosition
	GetRawSource() []byte
	GetParserNode() ASTParserNode
	GetContainer() *ASTNodeContainer
	GetParent() ASTNode
	SetParent(ASTNode)
	Nearest(func(ASTNode) bool) ASTNode
	NearestOrSelf(func(ASTNode) bool) ASTNode
	Children() []ASTNode
	AppendChild(ASTNode)
	String() string
	Kind() string
	Accept(visitor ASTVisitor) interface{}
	GetFilePath() string
	GetFileRange() *common.FileRange
	GetLanguage() string
	GetStringSource() string
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
