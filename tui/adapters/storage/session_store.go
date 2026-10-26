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
	"unicode/utf8"

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
	// toolSets caches the tool sets this store wrote or read, by reference.
	toolSets map[string][]dto.AvailableTool
	// OnDiscard is told each session Close discarded because it never
	// received a message, so what the console kept about it elsewhere (its
	// label) goes too.
	OnDiscard func(domain.SessionID)
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
	return &SessionStore{dir: stateDir, locks: make(map[domain.SessionID]*sessionLock), replace: os.Rename, toolSets: map[string][]dto.AvailableTool{}}, nil
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

// Release gives a session's lock back without closing the store: a session
// the console loaded only to look at it (the abandoned-instance sweep at
// start) stays openable by another console afterwards. An empty session is
// discarded, as Close does.
func (s *SessionStore) Release(id domain.SessionID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock := s.locks[id]
	if lock == nil {
		return nil
	}
	delete(s.locks, id)
	return s.releaseLock(id, lock)
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
	if record.ToolSnapshotRef, e = s.storeToolSet(record.ToolSnapshot); e != nil {
		return e
	}
	record.ToolSnapshot, record.Version = nil, storedSnapshotVersion
	// json.Marshal would escape <, > and & inside tool call arguments, so a
	// loaded session would no longer send the bytes the live one sent.
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if e = encoder.Encode(record); e != nil {
		return e
	}
	data := bytes.TrimSuffix(encoded.Bytes(), []byte("\n"))
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
	if e = s.writeInstance(session.Export().ID, session.Export().Instance); e != nil {
		return e
	}
	if e = s.writeConsole(session.Export().ID, notesOf(session.Export())); e != nil {
		return e
	}
	if e = s.writePolicy(session.Export().ID, session.Export().Policy); e != nil {
		return e
	}
	if e = s.writeExecution(session.Export().ID, session.Export().Budget, session.Export().Correlation, session.Export().Sandbox, session.Export().Routing); e != nil {
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
	if record.Version == storedSnapshotVersion {
		if record.ToolSnapshot != nil {
			return domain.Session{}, errors.New("a version 3 snapshot names its tools, it does not carry them")
		}
		if record.ToolSnapshotRef != "" {
			if record.ToolSnapshot, e = s.loadToolSet(record.ToolSnapshotRef); e != nil {
				return domain.Session{}, e
			}
			record.ToolSnapshotRef = ""
		}
	} else if record.ToolSnapshotRef != "" {
		return domain.Session{}, errors.New("only a version 3 snapshot names a tool set")
	}
	policy, e := s.readPolicy(id)
	if e != nil {
		return domain.Session{}, e
	}
	account, correlation, sandbox, routing, e := s.readExecution(id)
	if e != nil {
		return domain.Session{}, e
	}
	session, e := restoreSnapshot(record, s.preserveActive, s.readTimes(id), s.readMode(id), s.readCeremony(id), s.readInstance(id), s.readConsole(id))
	if e != nil {
		return domain.Session{}, e
	}
	if policy != nil {
		if e = session.SetPolicy(*policy); e != nil {
			return domain.Session{}, e
		}
	}
	if account != nil {
		if e = session.RestoreBudget(*account); e != nil {
			return domain.Session{}, e
		}
	}
	if correlation != nil {
		if e = session.SetCorrelation(*correlation); e != nil {
			return domain.Session{}, e
		}
	}
	if sandbox != nil {
		if e = session.SetSandbox(*sandbox); e != nil {
			return domain.Session{}, e
		}
	}
	if routing != nil {
		if e = session.SetRouting(*routing); e != nil {
			return domain.Session{}, e
		}
	}
	return session, nil
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
		summary := domain.SessionSummary{ID: state.ID, Workspace: state.Workspace, Model: state.Model, Status: state.Status, MessageCount: len(state.Messages), Mode: state.Mode}
		for _, message := range state.Messages {
			if message.Role == root.RoleUser {
				// The title is what the person typed: on 14 October 2026 a
				// one-prompt session was listed as "Di hola. [AXLR · recall]
				// Memory of project:ws6: …".
				summary.Title = root.Text(application.PersonTyped(string(message.Content)))
				break
			}
		}
		summary.Text = summaryText(state.Messages)
		if info, err := entry.Info(); err == nil {
			summary.UpdatedAt = info.ModTime()
		}
		result = append(result, summary)
	}
	return result, nil
}

// summaryText joins a session's prompts and replies for the picker's
// filter, up to domain.MaxSummaryTextBytes and never inside a character.
func summaryText(messages []root.Message) root.Text {
	var b strings.Builder
	for _, message := range messages {
		if message.Role != root.RoleUser && message.Role != root.RoleAssistant || message.Content == "" {
			continue
		}
		// Console notes are not what the person searches for.
		if strings.HasPrefix(string(message.Content), "[AXLR") {
			continue
		}
		text := application.PersonTyped(string(message.Content))
		if room := domain.MaxSummaryTextBytes - b.Len() - 1; len(text) > room {
			for room > 0 && !utf8.RuneStart(text[room]) {
				room--
			}
			text = text[:max(0, room)]
		}
		if text == "" {
			break
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(text)
	}
	return root.Text(b.String())
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
		errs = append(errs, s.releaseLock(id, lock))
	}
	return errors.Join(errs...)
}

// releaseLock gives a held session's lock back. A session that never
// received a message is discarded while its lock is still held, so every
// launch does not leave an empty session; nor does a /new the person left
// with Ctrl+O, now that leaving a session releases it (on 14 October 2026
// two empty sessions were left behind that way).
func (s *SessionStore) releaseLock(id domain.SessionID, lock *sessionLock) error {
	var errs []error
	empty := s.isEmpty(id)
	if empty {
		if err := os.Remove(filepath.Join(s.dir, string(id)+".json")); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
			empty = false
		}
	}
	errs = append(errs, lock.close())
	if empty {
		if s.OnDiscard != nil {
			s.OnDiscard(id)
		}
		_ = os.Remove(s.timesPath(id))
		_ = os.Remove(s.modePath(id))
		_ = os.Remove(s.ceremonyPath(id))
		_ = os.Remove(s.instancePath(id))
		_ = os.Remove(s.consolePath(id))
		_ = os.Remove(s.policyPath(id))
		_ = os.Remove(s.usagePath(id))
		_ = os.Remove(filepath.Join(s.dir, string(id)+".lock"))
	}
	return errors.Join(errs...)
}

func (s *SessionStore) isEmpty(id domain.SessionID) bool {
	session, err := s.read(id)
	if err != nil {
		return false
	}
	state := session.Export()
	return len(state.Messages) == 0 && state.Draft == "" && len(state.Activity) == 0 && len(state.ArchivedDrafts) == 0 && len(state.ConsoleLines) == 0
}
