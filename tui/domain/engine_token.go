package domain

import "unicode"

// engineToken reports whether an id an engine returned is one opaque
// token: 1 to 256 bytes, no space, no control character.
func engineToken(value string) bool {
	if value == "" || len(value) > 256 {
		return false
	}
	for _, r := range value {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}
