package ast

import "strings"

func formatSource(s string, n int) string {
	if n <= 0 || s == "" {
		return s
	}

	prefix := strings.Repeat(" ", n)
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = prefix + "| " + lines[i]
	}
	return strings.Join(lines, "\n")
}
