package domain

import (
	"errors"
	"strings"
	"unicode/utf8"
)

type Text string

func NewText(s string) (Text, error) {
	if !utf8.ValidString(s) || strings.ContainsRune(s, '\x00') {
		return "", errors.New("text must be UTF-8 without NUL")
	}
	return Text(s), nil
}
