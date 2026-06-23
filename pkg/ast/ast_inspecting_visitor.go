package ast

import (
	"reflect"
)

type InspectingVisitor struct{}

func NewInspectingVisitor() *InspectingVisitor {
	return &InspectingVisitor{}
}

func resolveNested(visitor ASTVisitor, t reflect.Type, v reflect.Value) *[]interface{} {
	res := []interface{}{}

	if t.Kind() == reflect.Pointer && v.IsNil() {
		return nil
	} else if accept := v.MethodByName("Accept"); accept.IsValid() {
		args := []reflect.Value{reflect.ValueOf(visitor)}

		fnRes := accept.Call(args)

		for _, element := range fnRes {
			sub := resolveNested(visitor, element.Type(), element)

			if sub != nil {
				if t.Kind() == reflect.Interface {
					concreteType := v.Elem().Type()
					if concreteType.Kind() == reflect.Pointer {
						concreteType = concreteType.Elem()
					}
					res = append(res, map[string]interface{}{concreteType.Name(): *sub})
				} else {
					res = append(res, *sub...)
				}
			}
		}
	} else if t.Kind() == reflect.Slice {
		for i := 0; i < v.Len(); i++ {
			element := v.Index(i)
			sub := resolveNested(visitor, element.Type(), element)

			if sub != nil {
				res = append(res, *sub...)
			}
		}
	} else if t.Kind() == reflect.Pointer {
		if !v.IsNil() {
			res = append(res, v.Interface())
		}
	} else {
		res = append(res, v.Interface())
	}

	return &res
}

func inspectingVisit(visitor ASTVisitor, node interface{}) []interface{} {
	if inspectMethod := reflect.ValueOf(node).MethodByName("Inspect"); inspectMethod.IsValid() {
		mt := inspectMethod.Type()
		if mt.NumIn() == 0 && mt.NumOut() == 1 {
			node = inspectMethod.Call(nil)[0].Interface()
		}
	}

	t := reflect.TypeOf(node)
	value := reflect.ValueOf(node)

	res := []interface{}{}

	if t.Kind() == reflect.Ptr && t.Elem().Kind() == reflect.Struct {
		t = t.Elem()
		value = value.Elem()
	}

	if !value.IsValid() {
		return res
	}

	if t.Kind() == reflect.Struct {
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			fieldType := field.Type
			fieldValue := value.FieldByName(field.Name)

			fmap := map[string]any{}
			if field.Anonymous && fieldType == reflect.TypeOf((*ASTNodeContainer)(nil)) {
				if astNode, ok := node.(ASTNode); ok {
					fmap[field.Name] = []interface{}{astNode.DebugPosition()}
					res = append(res, fmap)
				}
				continue
			}

			sub := resolveNested(visitor, fieldType, fieldValue)

			if sub != nil {
				fmap[field.Name] = sub
				res = append(res, fmap)
			}
		}
	} else {
		res = append(res, node)
	}

	return res
}

func (vis *InspectingVisitor) VisitModule(node *ASTModule) interface{} {
	return inspectingVisit(vis, node)
}

func (vis *InspectingVisitor) VisitImport(node *ASTImportStatement) interface{} {
	return inspectingVisit(vis, node)
}

func (vis *InspectingVisitor) VisitBlock(node *ASTBlock) interface{} {
	return inspectingVisit(vis, node)
}

func (vis *InspectingVisitor) VisitCallExpression(node *ASTCallExpression) interface{} {
	return inspectingVisit(vis, node)
}

func (vis *InspectingVisitor) VisitFuncExpression(node *ASTFuncExpression) interface{} {
	return inspectingVisit(vis, node)
}

func (vis *InspectingVisitor) VisitFuncArgument(node *ASTFuncArgument) interface{} {
	return inspectingVisit(vis, node)
}

func (vis *InspectingVisitor) VisitDeclaration(node *ASTDeclaration) interface{} {
	return inspectingVisit(vis, node)
}

func (vis *InspectingVisitor) VisitReturn(node *ASTReturnStatement) interface{} {
	return inspectingVisit(vis, node)
}

func (vis *InspectingVisitor) VisitSymbol(node *ASTSymbol) interface{} {
	return inspectingVisit(vis, node)
}
