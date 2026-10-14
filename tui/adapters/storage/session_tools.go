package storage

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/underpass-ai/AXLR/tui/dto"
)

// A session file used to carry the definitions of every tool the session
// offered: on 14 October 2026 an 8-message session weighed 329 KB, 338 KB of
// it tool schemas (4 KB of messages), rewritten on every save and repeated
// in every session. The store now keeps each distinct tool set once, under
// tools/<sha256>.json beside the sessions, and a session file (version 3)
// names it by its hash. A session bundle still carries its tools inline:
// it is read on another machine.
const (
	storedSnapshotVersion = 3
	toolsDirName          = "tools"
	// toolSetGrace keeps a tool set no session names yet: another console
	// may have written it and not yet replaced the session that names it.
	toolSetGrace = time.Hour
)

func (s *SessionStore) toolsDir() string { return filepath.Join(s.dir, toolsDirName) }

func (s *SessionStore) toolSetPath(ref string) string {
	return filepath.Join(s.toolsDir(), ref+".json")
}

// encodeToolSet is the tool set's stored bytes, whose sha256 names it.
func encodeToolSet(tools []dto.AvailableTool) ([]byte, error) {
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(tools); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(encoded.Bytes(), []byte("\n")), nil
}

func validToolSetRef(ref string) bool {
	if len(ref) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(ref)
	return err == nil && strings.ToLower(ref) == ref
}

// storeToolSet writes the tool set once and returns its reference; an
// empty set has none. A set already on disk is touched the first time this
// store uses it, so a sweep in another console does not take it as unused
// while this console's session is being replaced.
func (s *SessionStore) storeToolSet(tools []dto.AvailableTool) (string, error) {
	if len(tools) == 0 {
		return "", nil
	}
	data, err := encodeToolSet(tools)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	ref := hex.EncodeToString(sum[:])
	if _, seen := s.toolSets[ref]; seen {
		return ref, nil
	}
	if err := os.MkdirAll(s.toolsDir(), 0700); err != nil {
		return "", err
	}
	path := s.toolSetPath(ref)
	if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() {
		now := time.Now()
		_ = os.Chtimes(path, now, now)
	} else {
		file, err := os.CreateTemp(s.toolsDir(), ".tools-*")
		if err != nil {
			return "", err
		}
		defer os.Remove(file.Name())
		if err := file.Chmod(0600); err != nil {
			file.Close()
			return "", err
		}
		if _, err := file.Write(data); err != nil {
			file.Close()
			return "", err
		}
		if err := file.Sync(); err != nil {
			file.Close()
			return "", err
		}
		if err := file.Close(); err != nil {
			return "", err
		}
		if err := s.replace(file.Name(), path); err != nil {
			return "", err
		}
	}
	s.toolSets[ref] = tools
	return ref, nil
}

// loadToolSet reads a stored tool set and checks it still hashes to its
// name, so a session never offers tools that were changed on disk.
func (s *SessionStore) loadToolSet(ref string) ([]dto.AvailableTool, error) {
	if !validToolSetRef(ref) {
		return nil, errors.New("invalid tool set reference")
	}
	if tools, ok := s.toolSets[ref]; ok {
		return tools, nil
	}
	file, err := openNoFollow(s.toolSetPath(ref), os.O_RDONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("tool set %s: %w", ref[:12], err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("tool set must be a regular file")
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != ref {
		return nil, fmt.Errorf("tool set %s does not match its name", ref[:12])
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var tools []dto.AvailableTool
	if err := decoder.Decode(&tools); err != nil {
		return nil, err
	}
	s.toolSets[ref] = tools
	return tools, nil
}

// sweepToolSets removes the stored tool sets no session names any more,
// except those written or touched within toolSetGrace. A session file it
// cannot read keeps every set: the sweep only ever removes what it knows
// is unused.
func (s *SessionStore) sweepToolSets() error {
	entries, err := os.ReadDir(s.toolsDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	used := map[string]bool{}
	sessions, err := os.ReadDir(s.dir)
	if err != nil {
		return err
	}
	for _, entry := range sessions {
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") || strings.Count(name, ".") != 1 {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.dir, name))
		if err != nil {
			return nil
		}
		var named struct {
			Ref string `json:"tool_snapshot_ref"`
		}
		if err := json.Unmarshal(data, &named); err != nil {
			return nil
		}
		used[named.Ref] = true
	}
	var errs []error
	for _, entry := range entries {
		ref, ok := strings.CutSuffix(entry.Name(), ".json")
		if !ok || !validToolSetRef(ref) || used[ref] {
			continue
		}
		info, err := entry.Info()
		if err != nil || time.Since(info.ModTime()) < toolSetGrace {
			continue
		}
		if err := os.Remove(filepath.Join(s.toolsDir(), entry.Name())); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
		delete(s.toolSets, ref)
	}
	return errors.Join(errs...)
}
