package main

import (
	"fmt"

	fixture_dep1 "github.com/schahriar/captn/tests/fixtures/golang/multidep/pkg/dep1"
)

func main() {
	fmt.Println(fixture_dep1.GetExampleText())
}
