package domain

import (
	"errors"
	"path/filepath"
	"strings"
)

type RelativePath string

func NewRelativePath(s string) (RelativePath, error) {
	if s == "" || s == "." || filepath.IsAbs(s) || !filepath.IsLocal(s) || strings.ContainsRune(s, '\x00') {
		return "", errors.New("path must be relative to the workspace")
	}
	for _, part := range strings.Split(filepath.ToSlash(s), "/") {
		if part == ".." {
			return "", errors.New("path cannot contain parent traversal")
		}
	}
	return RelativePath(s), nil
}
