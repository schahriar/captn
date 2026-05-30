package ast

type ImportVisitor struct {
	Imports []*ASTImportStatement
}

func (vis *ImportVisitor) VisitModule(node *ASTModule) interface{} {
	return autoVisit(vis, node)
}

func (vis *ImportVisitor) VisitImport(node *ASTImportStatement) interface{} {
	vis.Imports = append(vis.Imports, node)
	return nil
}

func (vis *ImportVisitor) VisitBlock(node *ASTBlock) interface{} {
	return autoVisit(vis, node)
}

func (vis *ImportVisitor) VisitCallExpression(node *ASTCallExpression) interface{} {
	return autoVisit(vis, node)
}

func (vis *ImportVisitor) VisitFuncExpression(node *ASTFuncExpression) interface{} {
	return autoVisit(vis, node)
}

func (vis *ImportVisitor) VisitFuncArgument(node *ASTFuncArgument) interface{} {
	return autoVisit(vis, node)
}

func (vis *ImportVisitor) VisitDeclaration(node *ASTDeclaration) interface{} {
	return autoVisit(vis, node)
}

func (vis *ImportVisitor) VisitReturn(node *ASTReturnStatement) interface{} {
	return nil
}

func (vis *ImportVisitor) VisitSymbol(node *ASTSymbol) interface{} {
	return nil
}

// Conformance change
var _ ASTVisitor = &ImportVisitor{}
