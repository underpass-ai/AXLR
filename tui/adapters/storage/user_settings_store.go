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
	"time"
	"unicode/utf8"

	"github.com/underpass-ai/AXLR/tui/application"
)

const maxUserSettingsBytes = 64 << 10

// UserSettingsStore preserves unrelated keys when /model or /theme saves.
// Initial is used only until settings.json is created, allowing old preference
// files to be read without changing them.
type UserSettingsStore struct {
	Path    string
	Initial UserSettings
	mu      sync.Mutex
}

func NewUserSettingsStore(path string, initial UserSettings) (*UserSettingsStore, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("settings path must be absolute")
	}
	if err := initial.Validate(); err != nil {
		return nil, err
	}
	return &UserSettingsStore{Path: path, Initial: initial}, nil
}

func (s *UserSettingsStore) Load(ctx context.Context) (UserSettings, error) {
	if err := ctx.Err(); err != nil {
		return UserSettings{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read()
}

func (s *UserSettingsStore) read() (UserSettings, error) {
	file, err := openNoFollow(s.Path, os.O_RDONLY, 0)
	if errors.Is(err, os.ErrNotExist) {
		return s.Initial, nil
	}
	if err != nil {
		return UserSettings{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return UserSettings{}, err
	}
	if !info.Mode().IsRegular() || writableByOthers(info) || info.Size() > maxUserSettingsBytes {
		return UserSettings{}, errors.New("settings.json must be a regular, non-writable-by-others file of at most 64 KiB")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxUserSettingsBytes+1))
	if err != nil {
		return UserSettings{}, err
	}
	if len(data) > maxUserSettingsBytes || !utf8.Valid(data) {
		return UserSettings{}, errors.New("invalid settings.json size or UTF-8")
	}
	settings := DefaultUserSettings()
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&settings); err != nil || decoder.Decode(new(any)) != io.EOF {
		return UserSettings{}, errors.New("invalid settings.json JSON")
	}
	if err := settings.Validate(); err != nil {
		return UserSettings{}, err
	}
	return settings, nil
}

func (s *UserSettingsStore) update(ctx context.Context, change func(*UserSettings)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.Path), 0700); err != nil {
		return err
	}
	lock, err := openNoFollow(s.Path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	info, err := lock.Stat()
	if err != nil {
		return err
	}
	if !privateRegular(info) {
		return privateLockError("settings lock", s.Path+".lock", info)
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := tryLock(lock)
		if err == nil {
			break
		}
		if !lockBusy(err) {
			return err
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	settings, err := s.read()
	if err != nil {
		return err
	}
	change(&settings)
	if err := settings.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if len(data) > maxUserSettingsBytes {
		return errors.New("settings.json exceeds 64 KiB")
	}
	file, err := os.CreateTemp(filepath.Dir(s.Path), ".axlr-settings-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if err := file.Chmod(0600); err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), s.Path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(s.Path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return syncDirectoryFile(dir)
}

// Change applies one validated change to settings.json under its lock,
// creating the file when it does not exist; the setup writes its choices
// through it.
func (s *UserSettingsStore) Change(ctx context.Context, change func(*UserSettings)) error {
	return s.update(ctx, change)
}

func (s *UserSettingsStore) ModelPreference() application.ModelPreferencePort {
	return settingsModelPreference{store: s}
}

func (s *UserSettingsStore) UIPreference() application.UIPreferencePort {
	return settingsUIPreference{store: s}
}

func (s *UserSettingsStore) ModelFavorites() application.ModelFavoritesPort {
	return settingsModelFavorites{store: s}
}
