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
	"github.com/underpass-ai/AXLR/tui/dto"
)

const uiPreferenceFilename = "ui-preference.json"
const uiPreferenceVersion = 1
const maxUIPreferenceBytes = 4096

type UIPreferenceStore struct {
	dir string
	mu  sync.Mutex
}

var _ application.UIPreferencePort = (*UIPreferenceStore)(nil)

func NewUIPreferenceStore(dir string) (*UIPreferenceStore, error) {
	if !filepath.IsAbs(dir) {
		return nil, errors.New("UI preference directory must be absolute")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("UI preference directory must be a real directory")
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return nil, err
	}
	return &UIPreferenceStore{dir: dir}, nil
}

func (s *UIPreferenceStore) Load(ctx context.Context) (domain.UIPreferences, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defaults := domain.DefaultUIPreferences()
	if err := ctx.Err(); err != nil {
		return defaults, err
	}
	path := filepath.Join(s.dir, uiPreferenceFilename)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return defaults, nil
	}
	if err != nil {
		return defaults, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxUIPreferenceBytes {
		return defaults, errors.New("invalid UI preference file")
	}
	file, err := os.Open(path)
	if err != nil {
		return defaults, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxUIPreferenceBytes+1))
	if err != nil || len(data) > maxUIPreferenceBytes {
		return defaults, errors.New("invalid UI preference file")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var record dto.UIPreference
	if err := decoder.Decode(&record); err != nil || decoder.Decode(new(any)) != io.EOF || record.Version != uiPreferenceVersion {
		return defaults, errors.New("invalid UI preference file")
	}
	p := domain.UIPreferences{Theme: domain.ThemeID(record.Theme), Icons: domain.IconProfile(record.Icons), ReduceMotion: record.ReduceMotion}
	if err := p.Validate(); err != nil {
		return defaults, err
	}
	return p, nil
}

func (s *UIPreferenceStore) Save(ctx context.Context, p domain.UIPreferences) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := p.Validate(); err != nil {
		return err
	}
	path := filepath.Join(s.dir, uiPreferenceFilename)
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("UI preference target must be a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := json.Marshal(dto.UIPreference{Version: uiPreferenceVersion, Theme: string(p.Theme), Icons: string(p.Icons), ReduceMotion: p.ReduceMotion})
	if err != nil || len(data) > maxUIPreferenceBytes {
		return errors.New("invalid UI preference data")
	}
	file, err := os.CreateTemp(s.dir, ".ui-preference-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return err
	}
	dir, err := os.Open(s.dir)
	if err != nil {
		return err
	}
	defer dir.Close()
	return syncDirectoryFile(dir)
}
