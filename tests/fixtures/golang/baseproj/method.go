package fixture_main

type widget struct{}

func (w *widget) Describe(prefix string) string {
	return prefix
}
