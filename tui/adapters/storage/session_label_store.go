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
	"github.com/underpass-ai/AXLR/tui/domain"
)

const sessionLabelsVersion = 1
const maxSessionLabelsBytes = 256 << 10

type sessionLabelsRecord struct {
	Version int                          `json:"version"`
	Labels  map[string]sessionLabelEntry `json:"labels"`
}

type sessionLabelEntry struct {
	Title    string `json:"title,omitempty"`
	Archived bool   `json:"archived,omitempty"`
	About    string `json:"about,omitempty"`
}

// SessionLabelStore keeps session titles and archive flags in one file next
// to the sessions, so session snapshots keep their format.
type SessionLabelStore struct {
	path string
	mu   sync.Mutex
}

var _ application.SessionLabelsPort = (*SessionLabelStore)(nil)

func NewSessionLabelStore(path string) (*SessionLabelStore, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("session labels path must be absolute")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	return &SessionLabelStore{path: path}, nil
}

func (s *SessionLabelStore) Load(ctx context.Context) (map[domain.SessionID]domain.SessionLabel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	record, err := s.read()
	if err != nil {
		return nil, err
	}
	labels := make(map[domain.SessionID]domain.SessionLabel, len(record.Labels))
	for id, entry := range record.Labels {
		labels[domain.SessionID(id)] = domain.SessionLabel{Title: root.Text(entry.Title), Archived: entry.Archived, About: entry.About}
	}
	return labels, nil
}

func (s *SessionLabelStore) Set(ctx context.Context, id domain.SessionID, label domain.SessionLabel) error {
	_, err := s.update(ctx, id, label, false)
	return err
}

func (s *SessionLabelStore) Initialize(ctx context.Context, id domain.SessionID, label domain.SessionLabel) (domain.SessionLabel, error) {
	return s.update(ctx, id, label, true)
}

func (s *SessionLabelStore) update(ctx context.Context, id domain.SessionID, label domain.SessionLabel, initialize bool) (domain.SessionLabel, error) {
	if _, err := domain.NewSessionID(string(id)); err != nil {
		return domain.SessionLabel{}, err
	}
	if err := label.Validate(); err != nil {
		return domain.SessionLabel{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := acquireMCPConfigLock(ctx, s.path)
	if err != nil {
		return domain.SessionLabel{}, err
	}
	defer release()
	record, err := s.read()
	if err != nil {
		return domain.SessionLabel{}, err
	}
	if initialize {
		entry := record.Labels[string(id)]
		if entry.Title != "" {
			label.Title = root.Text(entry.Title)
		}
		if entry.About != "" {
			label.About = entry.About
		}
		label.Archived = entry.Archived
	} else if label.About == "" {
		// Picker edits may carry metadata loaded before the agent selected
		// its about. Renaming/archiving must not erase that newer binding.
		label.About = record.Labels[string(id)].About
	}
	if label == (domain.SessionLabel{}) {
		delete(record.Labels, string(id))
	} else {
		record.Labels[string(id)] = sessionLabelEntry{Title: string(label.Title), Archived: label.Archived, About: label.About}
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return domain.SessionLabel{}, err
	}
	if len(data) > maxSessionLabelsBytes {
		return domain.SessionLabel{}, errors.New("session labels file is full")
	}
	return label, s.write(ctx, data)
}

func (s *SessionLabelStore) read() (sessionLabelsRecord, error) {
	record := sessionLabelsRecord{Version: sessionLabelsVersion, Labels: map[string]sessionLabelEntry{}}
	file, err := openNoFollow(s.path, os.O_RDONLY, 0)
	if errors.Is(err, os.ErrNotExist) {
		return record, nil
	}
	if err != nil {
		return record, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return record, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxSessionLabelsBytes {
		return record, errors.New("invalid session labels file")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxSessionLabelsBytes+1))
	if err != nil {
		return record, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil || record.Version != sessionLabelsVersion {
		return record, errors.New("invalid session labels file")
	}
	if record.Labels == nil {
		record.Labels = map[string]sessionLabelEntry{}
	}
	return record, nil
}

func (s *SessionLabelStore) write(ctx context.Context, data []byte) error {
	dir := filepath.Dir(s.path)
	file, err := os.CreateTemp(dir, ".session-labels-*")
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
	if err := os.Rename(file.Name(), s.path); err != nil {
		return err
	}
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer directory.Close()
	return syncDirectoryFile(directory)
}
