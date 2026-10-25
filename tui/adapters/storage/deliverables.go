package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// Deliverables reads and writes axlr_deliver's files in the workspace: a
// delivered file is hashed where it is, never through a link out of the
// workspace; inline content is written under the deliverables folder, which
// ignores itself in git as the exports folder does.
type Deliverables struct{}

var _ application.DeliverablesPort = Deliverables{}

func (Deliverables) DeliverFile(ctx context.Context, workspace, relative string) (application.DeliveredFile, error) {
	if err := ctx.Err(); err != nil {
		return application.DeliveredFile{}, err
	}
	file, err := workspaceFile(workspace, relative)
	if err != nil {
		return application.DeliveredFile{}, err
	}
	handle, err := os.Open(file)
	if err != nil {
		return application.DeliveredFile{}, fmt.Errorf("%s cannot be read: %w", relative, err)
	}
	defer handle.Close()
	info, err := handle.Stat()
	if err != nil {
		return application.DeliveredFile{}, err
	}
	if !info.Mode().IsRegular() {
		return application.DeliveredFile{}, fmt.Errorf("%s is not a regular file", relative)
	}
	if info.Size() > domain.MaxDeliverableFileBytes {
		return application.DeliveredFile{}, fmt.Errorf("%s exceeds %d bytes", relative, domain.MaxDeliverableFileBytes)
	}
	digest := sha256.New()
	size, err := io.Copy(digest, io.LimitReader(handle, domain.MaxDeliverableFileBytes+1))
	if err != nil {
		return application.DeliveredFile{}, fmt.Errorf("%s cannot be read: %w", relative, err)
	}
	if size > domain.MaxDeliverableFileBytes {
		return application.DeliveredFile{}, fmt.Errorf("%s exceeds %d bytes", relative, domain.MaxDeliverableFileBytes)
	}
	return application.DeliveredFile{Size: size, SHA256: hex.EncodeToString(digest.Sum(nil))}, nil
}

func (Deliverables) WriteDeliverable(ctx context.Context, workspace, target string, content []byte) (application.DeliveredFile, error) {
	if err := ctx.Err(); err != nil {
		return application.DeliveredFile{}, err
	}
	if !strings.HasPrefix(target, domain.DeliverablesDirectory+"/") {
		return application.DeliveredFile{}, errors.New("an inline deliverable is written under " + domain.DeliverablesDirectory)
	}
	if len(content) > domain.MaxDeliverableInlineBytes {
		return application.DeliveredFile{}, fmt.Errorf("content exceeds %d bytes", domain.MaxDeliverableInlineBytes)
	}
	base, err := workspaceRoot(workspace)
	if err != nil {
		return application.DeliveredFile{}, err
	}
	relative, err := root.NewRelativePath(target)
	if err != nil {
		return application.DeliveredFile{}, err
	}
	// Each folder on the way is created or must be a folder, never a link:
	// a link planted at .axlr would write outside the workspace.
	dir := base
	parts := strings.Split(path.Dir(path.Clean(filepath.ToSlash(string(relative)))), "/")
	for _, part := range parts {
		dir = filepath.Join(dir, part)
		info, err := os.Lstat(dir)
		if errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(dir, 0755); err != nil && !errors.Is(err, os.ErrExist) {
				return application.DeliveredFile{}, err
			}
			continue
		}
		if err != nil {
			return application.DeliveredFile{}, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return application.DeliveredFile{}, fmt.Errorf("%s is not a folder of the workspace", filepath.ToSlash(strings.TrimPrefix(dir, base+string(filepath.Separator))))
		}
	}
	ignoreDeliverables(filepath.Join(base, filepath.FromSlash(domain.DeliverablesDirectory)))
	destination := filepath.Join(base, filepath.FromSlash(string(relative)))
	if info, err := os.Lstat(destination); err == nil && !info.Mode().IsRegular() {
		return application.DeliveredFile{}, fmt.Errorf("%s exists and is not a regular file", target)
	}
	temp, err := os.CreateTemp(filepath.Dir(destination), ".deliver-*")
	if err != nil {
		return application.DeliveredFile{}, err
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(content); err != nil {
		temp.Close()
		return application.DeliveredFile{}, err
	}
	if err := temp.Chmod(0644); err != nil {
		temp.Close()
		return application.DeliveredFile{}, err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return application.DeliveredFile{}, err
	}
	if err := temp.Close(); err != nil {
		return application.DeliveredFile{}, err
	}
	if err := os.Rename(temp.Name(), destination); err != nil {
		return application.DeliveredFile{}, err
	}
	digest := sha256.Sum256(content)
	return application.DeliveredFile{Size: int64(len(content)), SHA256: hex.EncodeToString(digest[:])}, nil
}

// ignoreDeliverables keeps the deliverables out of git status, as the
// exports folder does: they are handed over, not committed.
func ignoreDeliverables(dir string) {
	ignore := filepath.Join(dir, ".gitignore")
	if _, err := os.Lstat(ignore); errors.Is(err, os.ErrNotExist) {
		_ = os.WriteFile(ignore, []byte("*\n"), 0644)
	}
}

// workspaceRoot is the workspace as an absolute path with its links
// resolved.
func workspaceRoot(workspace string) (string, error) {
	if !filepath.IsAbs(workspace) {
		return "", errors.New("the workspace must be an absolute path")
	}
	return filepath.EvalSymlinks(workspace)
}

// workspaceFile resolves a workspace-relative path, links included, and
// refuses one that leaves the workspace.
func workspaceFile(workspace, relative string) (string, error) {
	base, err := workspaceRoot(workspace)
	if err != nil {
		return "", err
	}
	checked, err := root.NewRelativePath(relative)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(filepath.Join(base, filepath.FromSlash(string(checked))))
	if err != nil {
		return "", fmt.Errorf("%s cannot be delivered: %w", relative, err)
	}
	inside, err := filepath.Rel(base, resolved)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) || filepath.IsAbs(inside) {
		return "", fmt.Errorf("%s leaves the workspace", relative)
	}
	return resolved, nil
}
