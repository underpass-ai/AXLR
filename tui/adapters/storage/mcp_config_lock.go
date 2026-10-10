package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
	file, err := openNoFollow(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	release := func() { file.Close() }
	info, err := file.Stat()
	if err != nil {
		release()
		return nil, err
	}
	if !privateRegular(info) {
		release()
		return nil, privateLockError("MCP config lock", path+".lock", info)
	}
	for {
		if err := ctx.Err(); err != nil {
			release()
			return nil, err
		}
		err := tryLock(file)
		if err == nil {
			return release, nil
		}
		if !lockBusy(err) {
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
