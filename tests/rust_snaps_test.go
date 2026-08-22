package tests_test

import (
	"testing"
)

func TestRustSimpleFuncParse(t *testing.T) {
	checkSnapshot(t, "./fixtures/rust/baseproj/src/simple.rs")
}

func TestRustMultiDepParse(t *testing.T) {
	checkSnapshot(t, "./fixtures/rust/multidep/src/main.rs")
}

func TestRustMethodParse(t *testing.T) {
	checkSnapshot(t, "./fixtures/rust/baseproj/src/method.rs")
}
