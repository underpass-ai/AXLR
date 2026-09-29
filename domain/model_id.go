package domain

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

type ModelID string

func NewModelID(value string) (ModelID, error) {
	if !validModelIdentifier(value) {
		return "", errors.New("invalid model ID")
	}
	return ModelID(value), nil
}

func validModelIdentifier(value string) bool {
	if !utf8.ValidString(value) || strings.TrimSpace(value) == "" {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
