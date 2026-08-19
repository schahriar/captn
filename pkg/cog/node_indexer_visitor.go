package cog

import (
	"github.com/rdleal/intervalst/interval"
	"github.com/schahriar/captn/pkg/ast"
	"github.com/schahriar/captn/pkg/common"
	"github.com/schahriar/captn/pkg/knownerr"
)

type nodeIndexerVisitor struct {
	pf *COGFile
}

func NewnodeIndexerVisitor(pf *COGFile) *nodeIndexerVisitor {
	return &nodeIndexerVisitor{pf: pf}
}

func autoIndex(vis *nodeIndexerVisitor, node ast.ASTNode) interface{} {
	hash := ast.GetHash(node)
	pos := node.GetPosition()

	if coll, ok := vis.pf.lookupTable[hash]; ok {
		// This is a valid panic because it indicates a hash collision, which should not happen in a properly functioning implementation
		// We catch it and panic to report the issue as an implementation error, likely in a specific language parser
		panic(knownerr.HashCollision(node, hash, ast.GetHash(coll), coll))
	}

	vis.pf.lookupTable[hash] = node

	// Empty nodes can be skipped
	if pos.Start.BytePosition < pos.End.BytePosition {
		if err := vis.pf.intervals.Insert(pos.Start, pos.End, hash); err != nil {
			panic(knownerr.IntervalInsertError(node, err))
		}
	}

	return ast.AutoVisit(vis, node)
}

func (vis *nodeIndexerVisitor) VisitModule(node *ast.ASTModule) interface{} {
	return autoIndex(vis, node)
}

func (vis *nodeIndexerVisitor) VisitImport(node *ast.ASTImportStatement) interface{} {
	return autoIndex(vis, node)
}

func (vis *nodeIndexerVisitor) VisitBlock(node *ast.ASTBlock) interface{} {
	return autoIndex(vis, node)
}

func (vis *nodeIndexerVisitor) VisitCallExpression(node *ast.ASTCallExpression) interface{} {
	return autoIndex(vis, node)
}

func (vis *nodeIndexerVisitor) VisitFuncExpression(node *ast.ASTFuncExpression) interface{} {
	return autoIndex(vis, node)
}

func (vis *nodeIndexerVisitor) VisitFuncArgument(node *ast.ASTFuncArgument) interface{} {
	return autoIndex(vis, node)
}

func (vis *nodeIndexerVisitor) VisitDeclaration(node *ast.ASTDeclaration) interface{} {
	return autoIndex(vis, node)
}

func (vis *nodeIndexerVisitor) VisitReturn(node *ast.ASTReturnStatement) interface{} {
	return autoIndex(vis, node)
}

func (vis *nodeIndexerVisitor) VisitSymbol(node *ast.ASTSymbol) interface{} {
	return autoIndex(vis, node)
}

// Conformance check
var _ ast.ASTVisitor = (*nodeIndexerVisitor)(nil)

func (pf *COGFile) IndexNodes() {
	vis := NewnodeIndexerVisitor(pf)

	pf.lookupTable = map[common.HashType]ast.ASTNode{}
	pf.intervals = interval.NewSearchTree[common.HashType](common.CompareFilePosition)
	pf.isIndexed = false

	_ = vis.VisitModule(pf.Module)

	pf.isIndexed = true
}
