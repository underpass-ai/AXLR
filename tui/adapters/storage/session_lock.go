package storage

import (
	"fmt"
	"os"
	"syscall"
)

// sessionLock retains a stable inode across snapshot replacement. Never unlink it.
type sessionLock struct{ file *os.File }

func acquireLock(path string) (*sessionLock, error) {
	file, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return nil, e
	}
	if e = file.Chmod(0600); e != nil {
		file.Close()
		return nil, e
	}
	if e = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		file.Close()
		return nil, fmt.Errorf("session already has a writer: %w", e)
	}
	return &sessionLock{file: file}, nil
}
func (l *sessionLock) close() error { return l.file.Close() }
