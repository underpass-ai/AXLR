package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	Version   int                      `json:"version"`
	Workspace string                   `json:"workspace"`
	Tools     []application.ForgedTool `json:"tools"`
}

// ForgedToolStore keeps each workspace's forged tools: their files under
// <workspace>/.axlr/tools/<name>, written through os.Root so neither a path
// nor a symlink reaches outside the workspace, and their registry in Dir,
// the console's private state, one file per workspace. Only Forge writes the
// registry, so files that arrive with a checkout or are planted in the
// workspace are never registered tools.
type ForgedToolStore struct {
	Dir string
	mu  sync.Mutex
	// Now stamps forged_at; nil uses time.Now.
	Now func() time.Time
}

var _ application.ForgedToolsPort = (*ForgedToolStore)(nil)

// NewForgedToolStore keeps the registries in dir, created owner-only.
func NewForgedToolStore(dir string) (*ForgedToolStore, error) {
	if !filepath.IsAbs(dir) {
		return nil, errors.New("forged tools directory must be absolute")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return &ForgedToolStore{Dir: dir}, nil
}

func (s *ForgedToolStore) List(ctx context.Context, workspace string) ([]application.ForgedTool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	record, err := s.read(workspace)
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
	record, err := s.read(workspace)
	if err != nil {
		return application.ForgedTool{}, false, err
	}
	root, err := openWorkspace(workspace)
	if err != nil {
		return application.ForgedTool{}, false, err
	}
	defer root.Close()
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
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return application.ForgedTool{}, false, err
	}
	if len(data) > maxForgedRegistryBytes {
		return application.ForgedTool{}, false, fmt.Errorf("the registry of forged tools would exceed %d bytes", maxForgedRegistryBytes)
	}
	if err := writePrivateFile(ctx, s.registry(workspace), ".forged-*", append(data, '\n')); err != nil {
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

// registry is the workspace's registry file, named by a digest of its
// absolute path.
func (s *ForgedToolStore) registry(workspace string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(workspace)))
	return filepath.Join(s.Dir, hex.EncodeToString(sum[:16])+".json")
}

func (s *ForgedToolStore) read(workspace string) (forgedToolsRecord, error) {
	if !filepath.IsAbs(workspace) {
		return forgedToolsRecord{}, errors.New("workspace must be absolute")
	}
	if !filepath.IsAbs(s.Dir) {
		return forgedToolsRecord{}, errors.New("forged tools directory must be absolute")
	}
	empty := forgedToolsRecord{Version: forgedToolsVersion, Workspace: filepath.Clean(workspace)}
	file, err := os.Open(s.registry(workspace))
	if errors.Is(err, fs.ErrNotExist) {
		return empty, nil
	}
	if err != nil {
		return forgedToolsRecord{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxForgedRegistryBytes+1))
	if err != nil {
		return forgedToolsRecord{}, err
	}
	if len(data) > maxForgedRegistryBytes {
		return forgedToolsRecord{}, fmt.Errorf("the registry of forged tools exceeds %d bytes", maxForgedRegistryBytes)
	}
	var record forgedToolsRecord
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil || record.Version != forgedToolsVersion || record.Workspace != empty.Workspace {
		return forgedToolsRecord{}, fmt.Errorf("%s is not a version %d registry of this workspace's forged tools", s.registry(workspace), forgedToolsVersion)
	}
	return record, nil
}
