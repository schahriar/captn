package ast

import (
	"fmt"
	"runtime"
	"strings"

	errors "github.com/schahriar/captn/pkg/knownerr"
)

type ASTError struct {
	Value     interface{}
	Node      ASTNode
	CallStack *runtime.Frames
}

type ASTErrorStack struct {
	Stack []ASTNode
	Error error
}

func NewASTErrorStack() ASTErrorStack {
	return ASTErrorStack{
		Stack: []ASTNode{},
	}
}

func NewASTError(value interface{}, node ASTNode, callStack *runtime.Frames) ASTError {
	return ASTError{
		Value:     value,
		Node:      node,
		CallStack: callStack,
	}
}

func (estack ASTErrorStack) String() string {
	res := estack.Error.Error() + "\n\n"

	for i, node := range estack.Stack {
		pos := node.GetPosition()
		prefix := strings.Repeat(" ", i)

		res += fmt.Sprintf("%v> %v[%v]\n", prefix, node.String(), pos)

		if i == len(estack.Stack)-1 || i == 1 { // Print source n=1 and last node
			res += fmt.Sprintf("%v\n", formatSource(string(node.GetRawSource()), i+2))
		}
	}

	return res
}

func (ae ASTError) Error() string {

	astStack := ae.Stack().String()
	callStack := ""

	if ae.CallStack != nil {
		callStack += "-------- Stack Trace --------\n\n"

		for {
			frame, more := ae.CallStack.Next()
			callStack += fmt.Sprintf("%s\n\t%s:%d\n", frame.Function, frame.File, frame.Line)
			if !more {
				break
			}
		}

		callStack += "\n-------- ----------- --------\n"
	}

	return astStack + "\n" + callStack
}

func (ae ASTError) Stack() ASTErrorStack {
	return ae.StackWithMaxDepth(1000)
}

func (ae ASTError) StackWithMaxDepth(maxDepth int) ASTErrorStack {
	res := NewASTErrorStack()
	cur := ae

	for i := 0; i < maxDepth; i++ {
		res.Stack = append(res.Stack, cur.Node)

		if child, ok := cur.Value.(ASTError); ok && child.Node != nil {
			cur = child
		} else {
			if err, ok := cur.Value.(error); ok {
				res.Error = err
			}
			break
		}
	}

	return res
}

func ASTPanicBoundary(node ASTNode, fn func() interface{}) interface{} {
	// Using Golang's stack to reconstruct the tree (of AST) in the visitor pattern
	// and repanic with nodes attached at every Accept. This will allow detailed SyntaxError
	// and ImplementationError logs
	defer func() {
		if r := recover(); r != nil {
			if langerr, ok := r.(errors.Recoverable); ok && langerr.IsRecoverable() {
				pcs := make([]uintptr, 5)    // Top 5 items in the stack
				n := runtime.Callers(3, pcs) // Skip enough callers to get into the langerror panic producer
				pcs = pcs[:n]
				frames := runtime.CallersFrames(pcs)

				panic(NewASTError(r, node, frames))
			} else if astErr, ok := r.(ASTError); ok {
				panic(NewASTError(astErr, node, astErr.CallStack)) // Simply hoists the call stack up
			} else {
				panic(r)
			}
		}
	}()

	return fn()
}
