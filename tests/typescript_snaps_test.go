package tests_test

import (
	"testing"
)

func TestTSParserSimpleFuncParse(t *testing.T) {
	checkSnapshot(t, "./fixtures/typescript/baseproj/simple.ts")
}

func TestTSParserTypeFormsParse(t *testing.T) {
	checkSnapshot(t, "./fixtures/typescript/baseproj/types.ts")
}

func TestTSParserMultiDepParse(t *testing.T) {
	checkSnapshot(t, "./fixtures/typescript/multidep/main.ts")
}

func TestTSParserJavascriptParse(t *testing.T) {
	checkSnapshot(t, "./fixtures/typescript/baseproj/arrow.js")
}
