package storage

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/underpass-ai/AXLR/tui/application"
)

const forgedToolsVersion = 1

// maxForgedRegistryBytes bounds the registry file; maxForgedTools bounds the
// tools one workspace keeps.
const (
	maxForgedRegistryBytes = 1 << 20
	maxForgedTools         = 64
)

type forgedToolsRecord struct {
	Version int                      `json:"version"`
	Tools   []application.ForgedTool `json:"tools"`
}

// ForgedToolStore keeps each workspace's forged tools under
// <workspace>/.axlr/tools: one directory per tool and registry.json, which
// only Forge writes. Every access goes through os.Root, so neither a path nor
// a symlink reaches outside the workspace.
type ForgedToolStore struct {
	mu sync.Mutex
	// Now stamps forged_at; nil uses time.Now.
	Now func() time.Time
}

var _ application.ForgedToolsPort = (*ForgedToolStore)(nil)

func (s *ForgedToolStore) List(ctx context.Context, workspace string) ([]application.ForgedTool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	root, err := openWorkspace(workspace)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	record, err := readForged(root)
	if err != nil {
		return nil, err
	}
	return record.Tools, nil
}

func (s *ForgedToolStore) Forge(ctx context.Context, workspace string, tool application.ForgedTool, files []application.ForgedFile) (application.ForgedTool, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return application.ForgedTool{}, false, err
	}
	root, err := openWorkspace(workspace)
	if err != nil {
		return application.ForgedTool{}, false, err
	}
	defer root.Close()
	record, err := readForged(root)
	if err != nil {
		return application.ForgedTool{}, false, err
	}
	replaced := false
	kept := record.Tools[:0]
	for _, existing := range record.Tools {
		if existing.Name == tool.Name {
			replaced = true
			continue
		}
		kept = append(kept, existing)
	}
	if !replaced && len(kept) >= maxForgedTools {
		return application.ForgedTool{}, false, fmt.Errorf("this workspace already has %d forged tools; forge an existing one again instead", maxForgedTools)
	}
	dir := path.Join(application.ForgedToolsDir, tool.Name)
	// A new version starts from an empty directory, so no file of the old
	// one is left beside it.
	if err := root.RemoveAll(dir); err != nil {
		return application.ForgedTool{}, false, err
	}
	tool.Files = make(map[string]string, len(files))
	for _, file := range files {
		target := path.Join(dir, file.Path)
		if err := root.MkdirAll(path.Dir(target), 0o755); err != nil {
			return application.ForgedTool{}, false, err
		}
		if err := root.WriteFile(target, []byte(file.Content), 0o644); err != nil {
			return application.ForgedTool{}, false, err
		}
		sum := sha256.Sum256([]byte(file.Content))
		tool.Files[file.Path] = hex.EncodeToString(sum[:])
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	tool.ForgedAt = now().UTC().Format(time.RFC3339)
	record.Tools = append(kept, tool)
	sort.Slice(record.Tools, func(i, j int) bool { return record.Tools[i].Name < record.Tools[j].Name })
	if err := writeForged(root, record); err != nil {
		return application.ForgedTool{}, false, err
	}
	return tool, replaced, nil
}

func (s *ForgedToolStore) Verify(ctx context.Context, workspace string, tool application.ForgedTool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	root, err := openWorkspace(workspace)
	if err != nil {
		return err
	}
	defer root.Close()
	names := make([]string, 0, len(tool.Files))
	for name := range tool.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		data, err := root.ReadFile(path.Join(application.ForgedToolsDir, tool.Name, name))
		if err != nil {
			return fmt.Errorf("forged tool %q is missing %s; forge it again", tool.Name, name)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != tool.Files[name] {
			return fmt.Errorf("forged tool %q changed since it was forged (%s); forge it again so the code that runs is the code that was approved", tool.Name, name)
		}
	}
	return nil
}

func openWorkspace(workspace string) (*os.Root, error) {
	if !filepath.IsAbs(workspace) {
		return nil, errors.New("workspace must be absolute")
	}
	return os.OpenRoot(workspace)
}

func registryPath() string { return path.Join(application.ForgedToolsDir, "registry.json") }

func readForged(root *os.Root) (forgedToolsRecord, error) {
	data, err := root.ReadFile(registryPath())
	if errors.Is(err, fs.ErrNotExist) {
		return forgedToolsRecord{Version: forgedToolsVersion}, nil
	}
	if err != nil {
		return forgedToolsRecord{}, err
	}
	if len(data) > maxForgedRegistryBytes {
		return forgedToolsRecord{}, fmt.Errorf("%s exceeds %d bytes", registryPath(), maxForgedRegistryBytes)
	}
	var record forgedToolsRecord
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil || record.Version != forgedToolsVersion {
		return forgedToolsRecord{}, fmt.Errorf("%s is not a version %d registry of forged tools", registryPath(), forgedToolsVersion)
	}
	return record, nil
}

// writeForged replaces the registry atomically: a reader sees the old file or
// the new one.
func writeForged(root *os.Root, record forgedToolsRecord) error {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	if len(data) > maxForgedRegistryBytes {
		return fmt.Errorf("%s would exceed %d bytes", registryPath(), maxForgedRegistryBytes)
	}
	if err := root.MkdirAll(application.ForgedToolsDir, 0o755); err != nil {
		return err
	}
	var entropy [8]byte
	_, _ = rand.Read(entropy[:])
	temp := path.Join(application.ForgedToolsDir, ".registry-"+hex.EncodeToString(entropy[:]))
	if err := root.WriteFile(temp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	if err := root.Rename(temp, registryPath()); err != nil {
		_ = root.Remove(temp)
		return err
	}
	return nil
}
