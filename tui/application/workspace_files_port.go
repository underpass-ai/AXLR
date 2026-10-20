package application

import (
	"context"
	"errors"
)

// ErrNotText is wrapped by WorkspaceFilesPort.Read for a file that exists but
// is not UTF-8 text: the runtime reads text only, and a caller that only
// needs to know the file is there, or that it is opaque, tells it apart from
// a failed read with errors.Is.
var ErrNotText = errors.New("not UTF-8 text")

// WorkspaceFilesPort reads and writes workspace files for the console through
// the same runtime as the model's tools, so the workspace boundary holds.
type WorkspaceFilesPort interface {
	// Read returns at most maxBytes; found is false when the file is missing.
	// A file that is binary or not UTF-8 is an error wrapping ErrNotText.
	Read(ctx context.Context, path string, maxBytes int) (content []byte, found bool, err error)
	// Write creates or replaces the file.
	Write(ctx context.Context, path string, content []byte) error
	MakeDir(ctx context.Context, path string) error
}
