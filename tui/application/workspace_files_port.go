package application

import "context"

// WorkspaceFilesPort reads and writes workspace files for the console through
// the same runtime as the model's tools, so the workspace boundary holds.
type WorkspaceFilesPort interface {
	// Read returns at most maxBytes; found is false when the file is missing.
	Read(ctx context.Context, path string, maxBytes int) (content []byte, found bool, err error)
	// Write creates or replaces the file.
	Write(ctx context.Context, path string, content []byte) error
	MakeDir(ctx context.Context, path string) error
}
