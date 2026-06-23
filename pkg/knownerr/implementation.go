package knownerr

import "fmt"

type BaseErrorInterface interface {
	New(error) any
	error
}

type ImplementationError struct {
	Value error
}

func (se ImplementationError) IsRecoverable() bool {
	return true
}

func (se ImplementationError) Error() string {
	return se.Value.Error()
}

func NewImplementationError(val error) ImplementationError {
	return ImplementationError{
		Value: val,
	}
}

func (se ImplementationError) New(val error) any {
	return NewImplementationError(val)
}

func NewError[T BaseErrorInterface](err any, a ...any) T {
	var e T

	switch err := err.(type) {
	case error:
		return e.New(err).(T)
	case string:
		return e.New(fmt.Errorf(err, a...)).(T)
	default:
		return e.New(fmt.Errorf("BadError %v", err)).(T)
	}
}

func DoesNotAcceptChildren(t any) ImplementationError {
	return NewError[ImplementationError]("node %T does not accept children", t)
}

func HashCollision(node any, hash any, collisionHash any, collision any) ImplementationError {
	return NewError[ImplementationError]("hash collision detected for node %+v with hash %v = %v colliding with node %+v", node, hash, collisionHash, collision)
}

func IntervalInsertError(node any, err error) ImplementationError {
	return NewError[ImplementationError]("error inserting interval for node %+v: %w", node, err)
}

func UnsupportedFeature(feat string) ImplementationError {
	return NewError[ImplementationError]("unsupported feature: %v", feat)
}

func InvalidLocalType() ImplementationError {
	return NewError[ImplementationError]("invalid local type")
}
