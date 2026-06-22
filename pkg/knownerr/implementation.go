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
	return NewError[ImplementationError]("Node %T does not accept children", t)
}

func UnsupportedFeature(feat string) ImplementationError {
	return NewError[ImplementationError]("Unsupported feature: %v", feat)
}

func InvalidLocalType() ImplementationError {
	return NewError[ImplementationError]("Invalid local type")
}
