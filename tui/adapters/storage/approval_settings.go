package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

const maxApprovalSettingsBytes = 65536

type approvalSettingsFile struct {
	Version    int                   `json:"version"`
	Autonomous bool                  `json:"autonomous"`
	Allowed    []domain.ToolIdentity `json:"allowed"`
}

// ApprovalSettings stores exact tool identities. A saved choice applies on the
// next call too; the global autonomous switch includes every known tool.
type ApprovalSettings struct {
	mu       sync.RWMutex
	path     string
	fallback application.ToolApprovalPolicyPort
	data     approvalSettingsFile
}

var _ application.ApprovalSettingsPort = (*ApprovalSettings)(nil)

func NewApprovalSettings(path string, fallback application.ToolApprovalPolicyPort) (*ApprovalSettings, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("approval settings path must be absolute")
	}
	s := &ApprovalSettings{path: path, fallback: fallback, data: approvalSettingsFile{Version: 1}}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxApprovalSettingsBytes {
		return nil, errors.New("invalid approval settings file")
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) > maxApprovalSettingsBytes {
		return nil, errors.New("invalid approval settings file")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&s.data) != nil || decoder.Decode(new(any)) != io.EOF || s.data.Version != 1 {
		return nil, errors.New("invalid approval settings file")
	}
	for _, id := range s.data.Allowed {
		if id.Validate() != nil || id.Kind == domain.ToolKindHost {
			return nil, errors.New("invalid approved tool identity")
		}
	}
	return s, nil
}

func (s *ApprovalSettings) Autonomous() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.Autonomous
}

func (s *ApprovalSettings) Allowed() []domain.ToolIdentity {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]domain.ToolIdentity(nil), s.data.Allowed...)
}

func (s *ApprovalSettings) AutoApproves(id domain.ToolIdentity) bool {
	if id.Validate() != nil {
		return false
	}
	s.mu.RLock()
	full := s.data.Autonomous
	allowed := false
	for _, saved := range s.data.Allowed {
		if saved == id {
			allowed = true
			break
		}
	}
	s.mu.RUnlock()
	return full || allowed || (s.fallback != nil && s.fallback.AutoApproves(id))
}

func (s *ApprovalSettings) Allow(ctx context.Context, id domain.ToolIdentity) error {
	if err := id.Validate(); err != nil {
		return err
	}
	if id.Kind == domain.ToolKindHost {
		return errors.New("host wrappers cannot be added to always allow")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, saved := range s.data.Allowed {
		if saved == id {
			return nil
		}
	}
	next := s.data
	next.Allowed = append(append([]domain.ToolIdentity(nil), s.data.Allowed...), id)
	if err := s.save(ctx, next); err != nil {
		return err
	}
	s.data = next
	return nil
}

func (s *ApprovalSettings) SetAutonomous(ctx context.Context, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.Autonomous == enabled {
		return nil
	}
	next := s.data
	next.Autonomous = enabled
	if err := s.save(ctx, next); err != nil {
		return err
	}
	s.data = next
	return nil
}

func (s *ApprovalSettings) save(ctx context.Context, next approvalSettingsFile) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if info, err := os.Lstat(s.path); err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("approval settings target must be a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := json.Marshal(next)
	if err != nil || len(data) > maxApprovalSettingsBytes {
		return errors.New("approval settings too large")
	}
	f, err := os.CreateTemp(dir, ".approval-settings-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return os.Rename(f.Name(), s.path)
}
