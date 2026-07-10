package tui

import "fmt"

func Bold(s string) string {
	return ansiBold + s + ansiIntensityOff
}

func Dim(s string) string {
	return ansiDim + s + ansiIntensityOff
}

func Color(r, g, b int) func(string) string {
	prefix := fmt.Sprintf(ansiFgTrueColorFmt, r, g, b)
	return func(s string) string {
		return prefix + s + ansiFgReset
	}
}
