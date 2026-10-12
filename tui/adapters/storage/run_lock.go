package storage

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/underpass-ai/AXLR/tui/application"
)

// runTokenPattern is what a run token looks like: the hex of the random
// bytes a launch draws. Anything else names no lock file.
var runTokenPattern = regexp.MustCompile(`^[0-9a-f]{8,64}$`)

// RunLock is a console launch's hold on its run token: an exclusive lock on
// <dir>/<token>.lock kept for the launch's lifetime. Another console asks it
// whether the launch that recorded a plan or a job still runs: a lock it
// can take belongs to a launch that stopped, whatever way it stopped, since
// the system releases the lock of a process that dies.
type RunLock struct {
	dir   string
	token string
	mu    sync.Mutex
	file  *os.File
}

var _ application.RunLivenessPort = (*RunLock)(nil)

// HoldRun takes the lock of token in dir, creating dir owner-only, and
// removes the lock files of launches that stopped.
func HoldRun(dir, token string) (*RunLock, error) {
	if !filepath.IsAbs(dir) {
		return nil, errors.New("run lock directory must be absolute")
	}
	if !runTokenPattern.MatchString(token) {
		return nil, errors.New("invalid run token")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, token+".lock")
	for attempt := 0; ; attempt++ {
		if attempt == 50 {
			return nil, errors.New("could not hold the run lock " + path)
		}
		file, err := openNoFollow(path, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return nil, err
		}
		if err := tryLock(file); err != nil {
			file.Close()
			if !lockBusy(err) {
				return nil, err
			}
			// Another console is testing whether this new file's owner
			// is alive; it lets go at once.
			time.Sleep(10 * time.Millisecond)
			continue
		}
		// Another console's prune may have removed the file between the
		// open and the lock: a lock on an unlinked file proves nothing to
		// the others, so take the file that is there now.
		opened, openedErr := file.Stat()
		current, currentErr := os.Stat(path)
		if openedErr == nil && currentErr == nil && os.SameFile(opened, current) {
			lock := &RunLock{dir: dir, token: token, file: file}
			lock.prune()
			return lock, nil
		}
		file.Close()
	}
}

// Alive reports whether the launch that owns token still holds its lock.
// This launch is alive; a token without a lock file, or with one this
// launch can lock, belongs to a launch that stopped, and its file goes.
func (l *RunLock) Alive(token string) bool {
	if l == nil || !runTokenPattern.MatchString(token) {
		return false
	}
	if token == l.token {
		return true
	}
	path := filepath.Join(l.dir, token+".lock")
	file, err := openNoFollow(path, os.O_RDWR, 0)
	if err != nil {
		return false
	}
	err = tryLock(file)
	if err != nil && lockBusy(err) {
		file.Close()
		return true
	}
	file.Close()
	if err == nil {
		_ = os.Remove(path)
	}
	return false
}

// prune removes the lock files no launch holds.
func (l *RunLock) prune() {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if token, ok := strings.CutSuffix(entry.Name(), ".lock"); ok && token != l.token {
			l.Alive(token)
		}
	}
}

// Close releases the lock and removes this launch's file.
func (l *RunLock) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	// Removed while held, so no other console takes the file for a stopped
	// launch's in between; Windows removes it only once it is closed.
	path := filepath.Join(l.dir, l.token+".lock")
	removeErr := os.Remove(path)
	err := l.file.Close()
	l.file = nil
	if removeErr != nil {
		_ = os.Remove(path)
	}
	return err
}
