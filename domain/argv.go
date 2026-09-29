package domain

import (
	"errors"
	"strings"
)

type Argv []string

func NewArgv(args []string) (Argv, error) {
	for _, s := range args {
		if strings.ContainsRune(s, '\x00') {
			return nil, errors.New("argv contains NUL")
		}
	}
	return append(Argv{}, args...), nil
}
