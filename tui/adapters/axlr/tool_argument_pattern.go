package axlr

import (
	"errors"
	"strconv"
	"strings"
)

// JSON Schema patterns use ECMAScript escapes. Translate the BMP escapes and
// whitespace class used by the registered tool contracts before Go compiles
// them. Unsupported surrogate pairs are rejected rather than weakened.
func normalizeToolPattern(pattern string) (string, error) {
	const whitespace = `\x{0009}-\x{000D}\x{0020}\x{00A0}\x{1680}\x{2000}-\x{200A}\x{2028}\x{2029}\x{202F}\x{205F}\x{3000}\x{FEFF}`
	var out strings.Builder
	inClass := false
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		if c != '\\' || i+1 >= len(pattern) {
			out.WriteByte(c)
			if c == '[' {
				inClass = true
			} else if c == ']' {
				inClass = false
			}
			continue
		}
		i++
		switch pattern[i] {
		case 'u':
			if i+4 >= len(pattern) {
				return "", errors.New("incomplete Unicode pattern escape")
			}
			n, err := strconv.ParseUint(pattern[i+1:i+5], 16, 16)
			if err != nil || n >= 0xD800 && n <= 0xDFFF {
				return "", errors.New("unsupported Unicode pattern escape")
			}
			out.WriteString(`\x{` + strconv.FormatUint(n, 16) + `}`)
			i += 4
		case 's', 'S':
			if pattern[i] == 'S' && inClass {
				return "", errors.New("negated whitespace inside pattern class is unsupported")
			}
			if !inClass {
				out.WriteByte('[')
				if pattern[i] == 'S' {
					out.WriteByte('^')
				}
			}
			out.WriteString(whitespace)
			if !inClass {
				out.WriteByte(']')
			}
		default:
			out.WriteByte('\\')
			out.WriteByte(pattern[i])
		}
	}
	return out.String(), nil
}
