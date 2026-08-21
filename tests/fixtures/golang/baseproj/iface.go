package fixture_main

type Store interface {
	Get(key string) string
}

func use(s Store) string {
	return s.Get("x")
}
