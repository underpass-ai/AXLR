package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// maxImportBytes bounds a bundle read from disk.
const maxImportBytes = 64 << 20

// importSession reads a session bundle (/session export on another
// console) and saves it in this store, bound to the workspace, so the
// launch resumes it (decision I4 of the lifecycle integration design).
func importSession(ctx context.Context, path string, store application.SessionStorePort, labels application.SessionLabelsPort, workspace domain.Workspace, hostname string) (domain.SessionID, error) {
	if !filepath.IsAbs(path) {
		if absolute, err := filepath.Abs(path); err == nil {
			path = absolute
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("the bundle must be a regular file")
	}
	if info.Size() > maxImportBytes {
		return "", fmt.Errorf("the bundle exceeds %d bytes", maxImportBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	imported := application.ImportSessionUseCase{Codec: storage.SessionBundles{}, Store: store, Labels: labels, Workspace: workspace, Host: hostname, Source: path}
	session, _, err := imported.Execute(ctx, data)
	if err != nil {
		return "", fmt.Errorf("import %s: %w", path, err)
	}
	return session.ID(), nil
}

// memoryBundle adapts the KMP store's export to /memory save.
type memoryBundle func(ctx context.Context, about string) (application.BundleSaved, error)

func (b memoryBundle) Save(ctx context.Context, about string) (application.BundleSaved, error) {
	return b(ctx, about)
}
