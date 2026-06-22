package ast

type ImportVisitor struct {
	Imports []*ASTImportStatement
}

func NewImportVisitor() *ImportVisitor {
	return &ImportVisitor{}
}

func (vis *ImportVisitor) VisitModule(node *ASTModule) interface{} {
	return AutoVisit(vis, node)
}

func (vis *ImportVisitor) VisitImport(node *ASTImportStatement) interface{} {
	vis.Imports = append(vis.Imports, node)
	return nil
}

func (vis *ImportVisitor) VisitBlock(node *ASTBlock) interface{} {
	return AutoVisit(vis, node)
}

func (vis *ImportVisitor) VisitCallExpression(node *ASTCallExpression) interface{} {
	return AutoVisit(vis, node)
}

func (vis *ImportVisitor) VisitFuncExpression(node *ASTFuncExpression) interface{} {
	return AutoVisit(vis, node)
}

func (vis *ImportVisitor) VisitFuncArgument(node *ASTFuncArgument) interface{} {
	return AutoVisit(vis, node)
}

func (vis *ImportVisitor) VisitDeclaration(node *ASTDeclaration) interface{} {
	return AutoVisit(vis, node)
}

func (vis *ImportVisitor) VisitReturn(node *ASTReturnStatement) interface{} {
	return nil
}

func (vis *ImportVisitor) VisitSymbol(node *ASTSymbol) interface{} {
	return nil
}

// Conformance check
var _ ASTVisitor = (*ImportVisitor)(nil)
