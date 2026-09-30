package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// acquireMCPConfigLock serializes separate processes on a stable inode, never
// the configuration inode replaced by atomic writes.
func acquireMCPConfigLock(ctx context.Context, path string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(path) {
		return nil, errors.New("MCP config path must be absolute")
	}
	file, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return nil, err
	}
	release := func() { file.Close() }
	info, err := file.Stat()
	if err != nil {
		release()
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		release()
		return nil, errors.New("MCP config lock must be a private regular file")
	}
	for {
		if err := ctx.Err(); err != nil {
			release()
			return nil, err
		}
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return release, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			release()
			return nil, err
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			release()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}
