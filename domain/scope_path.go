package domain

import (
	"errors"
	"path"
	"path/filepath"
	"strings"
)

// ScopePath is the workspace directory or file a search or a listing starts
// from. Unlike RelativePath it may name the workspace itself, as "." or
// empty, and a trailing separator is dropped: "docs/" is the docs directory.
// It is slash separated and clean.
type ScopePath string

func NewScopePath(s string) (ScopePath, error) {
	slashed := filepath.ToSlash(s)
	if filepath.IsAbs(s) || strings.HasPrefix(slashed, "/") || strings.ContainsRune(s, '\x00') {
		return "", errors.New("path must be relative to the workspace")
	}
	for _, part := range strings.Split(slashed, "/") {
		if part == ".." {
			return "", errors.New("path cannot contain parent traversal")
		}
	}
	clean := path.Clean(slashed)
	if clean == "." {
		return ".", nil
	}
	if !filepath.IsLocal(filepath.FromSlash(clean)) {
		return "", errors.New("path must be relative to the workspace")
	}
	return ScopePath(clean), nil
}
