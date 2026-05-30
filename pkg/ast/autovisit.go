package ast

import "github.com/schahriar/captn/pkg/common"

func autoVisit(visitor ASTVisitor, node ASTNode) []interface{} {
	return common.Map(node.Children(), func(node ASTNode) interface{} {
		return node.Accept(visitor)
	})
}
