package storage

import (
	"fmt"
	"os"

	"github.com/underpass-ai/AXLR/tui/domain"
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
		return nil, fmt.Errorf("%w: %w", domain.ErrSessionHeld, e)
	}
	return &sessionLock{file: file}, nil
}
func (l *sessionLock) close() error { return l.file.Close() }
