package knownerr

type ProviderOutputError struct {
	Value error
}

func (se ProviderOutputError) IsRecoverable() bool {
	return true
}

func (se ProviderOutputError) Error() string {
	return se.Value.Error()
}

func (se ProviderOutputError) Unwrap() error {
	return se.Value
}

func NewProviderOutputError(val error) ProviderOutputError {
	return ProviderOutputError{
		Value: val,
	}
}

func (se ProviderOutputError) New(val error) any {
	return NewProviderOutputError(val)
}
