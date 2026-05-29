package knownerr

type Recoverable interface {
	IsRecoverable() bool
}
