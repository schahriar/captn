package sample

type Banned struct{}

func NewBanned() *Banned {
	return &Banned{}
}

func f() {
	_ = Banned{} // want "direct instantiation of Banned is banned; use NewBanned constructor"

	// banstructlit:ignore
	_ = Banned{}

	_ = new(Banned) // want "new\\(Banned\\) is banned; use NewBanned constructor"

	// banstructlit:ignore
	_ = new(Banned)
}

func g() {
	var values []Banned

	// banstructlit:ignore
	values = append(
		values,
		Banned{},
	)

	var pointers []*Banned

	// banstructlit:ignore
	pointers = append(
		pointers,
		new(Banned),
	)
}
