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

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/dto"
)

const modelPreferenceVersion = 1
const modelPreferenceFilename = "model-preference.json"
const maxModelPreferenceBytes = 4096

type ModelPreferenceStore struct {
	dir string
	mu  sync.Mutex
}

var _ application.ModelPreferencePort = (*ModelPreferenceStore)(nil)

func NewModelPreferenceStore(dir string) (*ModelPreferenceStore, error) {
	if !filepath.IsAbs(dir) {
		return nil, errors.New("model preference directory must be absolute")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("model preference directory must be a real directory")
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return nil, err
	}
	return &ModelPreferenceStore{dir: dir}, nil
}

func (s *ModelPreferenceStore) Load(ctx context.Context) (root.ModelID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	path := filepath.Join(s.dir, modelPreferenceFilename)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() > maxModelPreferenceBytes {
		return "", errors.New("invalid model preference file")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxModelPreferenceBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxModelPreferenceBytes {
		return "", errors.New("model preference file too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var record dto.ModelPreference
	if err := decoder.Decode(&record); err != nil {
		return "", errors.New("invalid model preference file")
	}
	if decoder.Decode(new(any)) != io.EOF || record.Version != modelPreferenceVersion {
		return "", errors.New("invalid model preference file")
	}
	model, err := root.NewModelID(record.Model)
	if err != nil {
		return "", errors.New("invalid model preference file")
	}
	return model, nil
}

func (s *ModelPreferenceStore) Save(ctx context.Context, model root.ModelID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := root.NewModelID(string(model)); err != nil {
		return err
	}
	path := filepath.Join(s.dir, modelPreferenceFilename)
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return errors.New("model preference target must be a regular file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := json.Marshal(dto.ModelPreference{Version: modelPreferenceVersion, Model: string(model)})
	if err != nil {
		return err
	}
	if len(data) > maxModelPreferenceBytes {
		return errors.New("model preference file too large")
	}
	file, err := os.CreateTemp(s.dir, ".model-preference-*")
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
	return dir.Sync()
}
