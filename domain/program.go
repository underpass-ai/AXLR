package domain

import (
	"errors"
	"strings"
)

type Program string

func NewProgram(s string) (Program, error) {
	if s == "" || strings.ContainsRune(s, '\x00') {
		return "", errors.New("invalid program")
	}
	return Program(s), nil
}
