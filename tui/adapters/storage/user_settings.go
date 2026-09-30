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
	"syscall"
	"time"
	"unicode/utf8"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

const maxUserSettingsBytes = 64 << 10

// UserSettings is the editable, user-facing configuration in settings.json.
// An empty model means that the console opens without a selected model.
type UserSettings struct {
	Model        string                     `json:"model"`
	Language     string                     `json:"language"`
	Theme        string                     `json:"theme"`
	Icons        string                     `json:"icons"`
	ReduceMotion bool                       `json:"reduce_motion"`
	Extra        map[string]json.RawMessage `json:"-"`
}

// Keep settings added by newer AXLR versions when an older console saves a
// model or theme. JSON values remain raw until the version that owns them.
func (s *UserSettings) UnmarshalJSON(data []byte) error {
	type known UserSettings
	value := known(DefaultUserSettings())
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if fields == nil {
		return errors.New("settings.json must contain a JSON object")
	}
	for _, key := range []string{"model", "language", "theme", "icons", "reduce_motion"} {
		delete(fields, key)
	}
	*s = UserSettings(value)
	s.Extra = fields
	return nil
}

func (s UserSettings) MarshalJSON() ([]byte, error) {
	type known UserSettings
	data, err := json.Marshal(known(s))
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, err
	}
	for key, value := range s.Extra {
		if _, owned := fields[key]; !owned {
			fields[key] = value
		}
	}
	return json.Marshal(fields)
}

func DefaultUserSettings() UserSettings {
	return UserSettings{Language: "en", Theme: string(domain.ThemeAuto), Icons: string(domain.IconsSafe)}
}

func (s UserSettings) Validate() error {
	if s.Model != "" {
		if _, err := root.NewModelID(s.Model); err != nil {
			return errors.New("invalid settings model")
		}
	}
	if s.Language != "en" && s.Language != "es" {
		return errors.New("settings language must be en or es")
	}
	return (domain.UIPreferences{Theme: domain.ThemeID(s.Theme), Icons: domain.IconProfile(s.Icons), ReduceMotion: s.ReduceMotion}).Validate()
}

func (s UserSettings) UIPreferences() domain.UIPreferences {
	return domain.UIPreferences{Theme: domain.ThemeID(s.Theme), Icons: domain.IconProfile(s.Icons), ReduceMotion: s.ReduceMotion}
}

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
	file, err := os.OpenFile(s.Path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
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
	if !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 || info.Size() > maxUserSettingsBytes {
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
	lock, err := os.OpenFile(s.Path+".lock", os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	info, err := lock.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return errors.New("settings lock must be a private regular file")
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
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
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
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
	return dir.Sync()
}

type settingsModelPreference struct{ store *UserSettingsStore }

var _ application.ModelPreferencePort = settingsModelPreference{}

func (s *UserSettingsStore) ModelPreference() application.ModelPreferencePort {
	return settingsModelPreference{store: s}
}

func (p settingsModelPreference) Load(ctx context.Context) (root.ModelID, error) {
	s, err := p.store.Load(ctx)
	return root.ModelID(s.Model), err
}

func (p settingsModelPreference) Save(ctx context.Context, model root.ModelID) error {
	if _, err := root.NewModelID(string(model)); err != nil {
		return err
	}
	return p.store.update(ctx, func(s *UserSettings) { s.Model = string(model) })
}

type settingsUIPreference struct{ store *UserSettingsStore }

var _ application.UIPreferencePort = settingsUIPreference{}

func (s *UserSettingsStore) UIPreference() application.UIPreferencePort {
	return settingsUIPreference{store: s}
}

func (p settingsUIPreference) Load(ctx context.Context) (domain.UIPreferences, error) {
	s, err := p.store.Load(ctx)
	return s.UIPreferences(), err
}

func (p settingsUIPreference) Save(ctx context.Context, ui domain.UIPreferences) error {
	if err := ui.Validate(); err != nil {
		return err
	}
	return p.store.update(ctx, func(s *UserSettings) {
		s.Theme = string(ui.Theme)
		s.Icons = string(ui.Icons)
		s.ReduceMotion = ui.ReduceMotion
	})
}
