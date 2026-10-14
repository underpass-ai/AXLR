package storage

import (
	"os"
	"path/filepath"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// HeldElsewhere reports whether another console has the saved session id
// open: it holds the session's writer lock. A session this store holds is
// not elsewhere. The probe takes the lock without creating its file and
// lets it go at once; flock conflicts between two descriptors of one
// process too, so the store's own locks are looked up first. On 14
// October 2026 Ctrl+O listed such a session like any other, and Enter on it
// failed with "Error:…" cut short.
func (s *SessionStore) HeldElsewhere(id domain.SessionID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.locks[id] != nil {
		return false
	}
	if _, err := domain.NewSessionID(string(id)); err != nil {
		return false
	}
	file, err := openNoFollow(filepath.Join(s.dir, string(id)+".lock"), os.O_RDWR, 0)
	if err != nil {
		// No lock file: nobody ever opened it for writing.
		return false
	}
	defer file.Close()
	return tryLock(file) != nil
}
