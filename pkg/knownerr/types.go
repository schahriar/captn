package knownerr

type TypeError struct {
	Value error
}

func (se TypeError) IsRecoverable() bool {
	return true
}

func (se TypeError) Error() string {
	return se.Value.Error()
}

func (se TypeError) New(val error) any {
	return TypeError{
		Value: val,
	}
}

func UnresolvedType() TypeError {
	return NewError[TypeError]("Unknown type for reference")
}

func UnresolvedTypeOfReference() TypeError {
	return NewError[TypeError]("Reference type is unresolved")
}
