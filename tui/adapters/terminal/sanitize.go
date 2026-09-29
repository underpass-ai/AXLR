package terminal

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// Sanitize removes terminal commands and controls while retaining readable text.
func Sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) || r == '\u202e' || r == '\u202d' || (r >= '\u2066' && r <= '\u2069') {
			return -1
		}
		return r
	}, ansi.Strip(s))
}

func singleLine(s string) string { return strings.Join(strings.Fields(Sanitize(s)), " ") }
