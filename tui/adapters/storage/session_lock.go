package storage

import (
	"fmt"
	"os"
)

// sessionLock retains a stable inode across snapshot replacement. Never unlink it.
type sessionLock struct{ file *os.File }

func acquireLock(path string) (*sessionLock, error) {
	file, e := openNoFollow(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = file.Chmod(0600); e != nil {
		file.Close()
		return nil, e
	}
	if e = tryLock(file); e != nil {
		file.Close()
		return nil, fmt.Errorf("session already has a writer: %w", e)
	}
	return &sessionLock{file: file}, nil
}
func (l *sessionLock) close() error { return l.file.Close() }
