package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

var _ application.SessionDeletePort = (*SessionStore)(nil)

// Delete removes a saved session: its snapshot, the files beside it and
// its lock. It takes the session's writer lock first, unless this store
// already holds it, so a session another console has open is refused.
// Which session the person has on screen is the caller's to refuse.
func (s *SessionStore) Delete(ctx context.Context, id domain.SessionID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.check(ctx); err != nil {
		return err
	}
	if _, err := domain.NewSessionID(string(id)); err != nil {
		return err
	}
	snapshot := filepath.Join(s.dir, string(id)+".json")
	if _, err := os.Lstat(snapshot); err != nil {
		return err
	}
	lock := s.locks[id]
	if lock == nil {
		var err error
		if lock, err = acquireLock(filepath.Join(s.dir, string(id)+".lock")); err != nil {
			return fmt.Errorf("the session is open in another console: %w", err)
		}
	}
	err := os.Remove(snapshot)
	if err == nil {
		for _, path := range []string{s.timesPath(id), s.modePath(id), s.ceremonyPath(id), s.instancePath(id), s.consolePath(id), s.usagePath(id)} {
			if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
				err = errors.Join(err, removeErr)
			}
		}
	}
	// The lock goes with the session, removed while still held as Close
	// removes an empty session's.
	if err == nil {
		_ = os.Remove(filepath.Join(s.dir, string(id)+".lock"))
	}
	delete(s.locks, id)
	return errors.Join(err, lock.close())
}
