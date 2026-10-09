package domain

import (
	"errors"
	"path/filepath"
	"strings"
)

type WorkingDirectory string

func NewWorkingDirectory(s string) (WorkingDirectory, error) {
	// A directory may be named with a trailing separator, unlike a RelativePath.
	if trimmed := strings.TrimRight(s, "/"+string(filepath.Separator)); trimmed != "" {
		s = trimmed
	}
	if s == "" || s == "." {
		return ".", nil
	}
	p, err := NewRelativePath(s)
	if err != nil {
		return "", errors.New("invalid working directory")
	}
	return WorkingDirectory(p), nil
}
