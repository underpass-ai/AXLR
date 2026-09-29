package domain

import (
	"errors"
	"path/filepath"

	axlr "github.com/underpass-ai/AXLR/domain"
)

type Workspace string

// NewWorkspace validates identity only; existence belongs to the runtime adapter.
func NewWorkspace(value string) (Workspace, error) {
	if _, err := axlr.NewText(value); err != nil {
		return "", err
	}
	if !filepath.IsAbs(value) {
		return "", errors.New("workspace must be absolute")
	}
	return Workspace(value), nil
}
