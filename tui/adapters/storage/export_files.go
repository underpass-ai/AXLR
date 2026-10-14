package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/underpass-ai/AXLR/tui/application"
)

// ExportFiles writes the person's session exports: an owner-only file in a
// directory created owner-only when missing, never over an existing file or
// through a link.
type ExportFiles struct{}

var _ application.ExportFilePort = ExportFiles{}

func (ExportFiles) Create(ctx context.Context, path string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !filepath.IsAbs(path) {
		return errors.New("export path must be absolute")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if strings.HasSuffix(dir, string(filepath.Separator)+filepath.FromSlash(application.ExportDirectory)) {
		// The default directory ignores itself, as the .kernel store does:
		// on 14 October 2026 git status offered the exported transcripts
		// to the next commit.
		ignore := filepath.Join(dir, ".gitignore")
		if _, err := os.Lstat(ignore); errors.Is(err, os.ErrNotExist) {
			_ = os.WriteFile(ignore, []byte("*\n"), 0600)
		}
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return exportExists{path}
	}
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		_ = os.Remove(path)
		return err
	}
	return file.Close()
}

// exportExists refuses an export over an existing file; it is an
// os.ErrExist, so a default name can be tried again with a suffix.
type exportExists struct{ path string }

func (e exportExists) Error() string {
	return fmt.Sprintf("%s already exists; export to another path", e.path)
}

func (exportExists) Unwrap() error { return os.ErrExist }
