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
	// The file adapter splits a path into directory and base name, so "notes/"
	// would address notes/notes.
	if strings.HasSuffix(filepath.ToSlash(s), "/") {
		return "", errors.New("path must not end with a separator")
	}
	for _, part := range strings.Split(filepath.ToSlash(s), "/") {
		if part == ".." {
			return "", errors.New("path cannot contain parent traversal")
		}
	}
	return RelativePath(s), nil
}
