package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
	"github.com/underpass-ai/AXLR/tui/dto"
)

// SessionStore owns exclusive writer locks for sessions it saves or loads.
// List reads atomic snapshots without claiming sessions. Close releases all locks.
type SessionStore struct {
	mu             sync.Mutex
	dir            string
	locks          map[domain.SessionID]*sessionLock
	closed         bool
	preserveActive bool
	replace        func(string, string) error
}

var _ application.SessionStorePort = (*SessionStore)(nil)

func New(stateDir string) (*SessionStore, error) {
	if stateDir == "" {
		base := os.Getenv("XDG_STATE_HOME")
		if base == "" || !filepath.IsAbs(base) {
			home, e := os.UserHomeDir()
			if e != nil {
				return nil, e
			}
			base = filepath.Join(home, ".local", "state")
		}
		stateDir = filepath.Join(base, "axlr", "sessions")
	}
	if e := os.MkdirAll(stateDir, 0700); e != nil {
		return nil, e
	}
	info, e := os.Lstat(stateDir)
	if e != nil {
		return nil, e
	}
	if !info.IsDir() {
		return nil, errors.New("session directory must be a real directory")
	}
	if e = os.Chmod(stateDir, 0700); e != nil {
		return nil, e
	}
	return &SessionStore{dir: stateDir, locks: make(map[domain.SessionID]*sessionLock), replace: os.Rename}, nil
}

// NewService preserves active states on ordinary reads. The service performs
// recovery once at startup, then keeps live turn status visible to clients.
func NewService(stateDir string) (*SessionStore, error) {
	store, err := New(stateDir)
	if err != nil {
		return nil, err
	}
	store.preserveActive = true
	return store, nil
}
func (s *SessionStore) check(ctx context.Context) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if s.closed {
		return errors.New("session store is closed")
	}
	return nil
}
func (s *SessionStore) claim(id domain.SessionID) error {
	if _, e := domain.NewSessionID(string(id)); e != nil {
		return e
	}
	if s.locks[id] != nil {
		return nil
	}
	lock, e := acquireLock(filepath.Join(s.dir, string(id)+".lock"))
	if e != nil {
		return e
	}
	s.locks[id] = lock
	return nil
}
func (s *SessionStore) Save(ctx context.Context, session domain.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.check(ctx); e != nil {
		return e
	}
	record, e := snapshot(session)
	if e != nil {
		return e
	}
	if e = s.claim(session.Export().ID); e != nil {
		return e
	}
	data, e := json.Marshal(record)
	if e != nil {
		return e
	}
	file, e := os.CreateTemp(s.dir, ".snapshot-*")
	if e != nil {
		return e
	}
	defer os.Remove(file.Name())
	if _, e = file.Write(data); e != nil {
		file.Close()
		return e
	}
	if e = file.Sync(); e != nil {
		file.Close()
		return e
	}
	if e = file.Close(); e != nil {
		return e
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	if e = s.replace(file.Name(), filepath.Join(s.dir, record.ID+".json")); e != nil {
		return e
	}
	if e = s.writeTimes(session.Export().ID, session.Export().MessageTimes); e != nil {
		return e
	}
	if e = s.writeMode(session.Export().ID, session.Mode()); e != nil {
		return e
	}
	if e = s.writeCeremony(session.Export().ID, session.Export().Ceremony); e != nil {
		return e
	}
	dir, e := os.Open(s.dir)
	if e != nil {
		return e
	}
	defer dir.Close()
	return syncDirectoryFile(dir)
}
func (s *SessionStore) Load(ctx context.Context, id domain.SessionID) (domain.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.check(ctx); e != nil {
		return domain.Session{}, e
	}
	if e := s.claim(id); e != nil {
		return domain.Session{}, e
	}
	return s.read(id)
}
func (s *SessionStore) read(id domain.SessionID) (domain.Session, error) {
	file, e := openNoFollow(filepath.Join(s.dir, string(id)+".json"), os.O_RDONLY, 0)
	if e != nil {
		return domain.Session{}, e
	}
	defer file.Close()
	info, e := file.Stat()
	if e != nil {
		return domain.Session{}, e
	}
	if !info.Mode().IsRegular() {
		return domain.Session{}, errors.New("snapshot must be a regular file")
	}
	if e = file.Chmod(0600); e != nil {
		return domain.Session{}, e
	}
	data, e := io.ReadAll(file)
	if e != nil {
		return domain.Session{}, e
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var record dto.SessionSnapshot
	if e = decoder.Decode(&record); e != nil {
		return domain.Session{}, e
	}
	if e = decoder.Decode(new(any)); e != io.EOF {
		return domain.Session{}, errors.New("trailing snapshot data")
	}
	if record.ID != string(id) {
		return domain.Session{}, errors.New("snapshot ID does not match filename")
	}
	return restoreSnapshot(record, s.preserveActive, s.readTimes(id), s.readMode(id), s.readCeremony(id))
}

// List is deterministic by session ID; malformed snapshots surface as errors.
func (s *SessionStore) List(ctx context.Context) ([]domain.SessionSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.check(ctx); e != nil {
		return nil, e
	}
	entries, e := os.ReadDir(s.dir)
	if e != nil {
		return nil, e
	}
	var result []domain.SessionSummary
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id, e := domain.NewSessionID(strings.TrimSuffix(entry.Name(), ".json"))
		if e != nil {
			continue
		}
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		session, e := s.read(id)
		if e != nil {
			return nil, fmt.Errorf("session %s: %w", id, e)
		}
		state := session.Export()
		summary := domain.SessionSummary{ID: state.ID, Workspace: state.Workspace, Model: state.Model, Status: state.Status, MessageCount: len(state.Messages)}
		for _, message := range state.Messages {
			if message.Role == root.RoleUser {
				summary.Title = message.Content
				break
			}
		}
		if info, err := entry.Info(); err == nil {
			summary.UpdatedAt = info.ModTime()
		}
		result = append(result, summary)
	}
	return result, nil
}
func (s *SessionStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	var errs []error
	for id, lock := range s.locks {
		// A session that never received a message is discarded while its lock
		// is still held, so every launch does not leave an empty session.
		empty := s.isEmpty(id)
		if empty {
			if err := os.Remove(filepath.Join(s.dir, string(id)+".json")); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, err)
				empty = false
			}
		}
		errs = append(errs, lock.close())
		if empty {
			_ = os.Remove(s.timesPath(id))
			_ = os.Remove(s.modePath(id))
			_ = os.Remove(s.ceremonyPath(id))
			_ = os.Remove(filepath.Join(s.dir, string(id)+".lock"))
		}
	}
	return errors.Join(errs...)
}

func (s *SessionStore) isEmpty(id domain.SessionID) bool {
	session, err := s.read(id)
	if err != nil {
		return false
	}
	state := session.Export()
	return len(state.Messages) == 0 && state.Draft == "" && len(state.Activity) == 0 && len(state.ArchivedDrafts) == 0
}
