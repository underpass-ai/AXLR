//go:build !windows

package storage

import (
	"errors"
	"os"
	"syscall"
)

func openNoFollow(path string, flags int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(path, flags|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, perm)
}

func privateRegular(info os.FileInfo) bool {
	return info.Mode().IsRegular() && info.Mode().Perm()&0077 == 0
}

func tryLock(file *os.File) error {
	return syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

func lockBusy(err error) bool {
	return errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN)
}

func syncDirectoryFile(file *os.File) error { return file.Sync() }

func writableByOthers(info os.FileInfo) bool { return info.Mode().Perm()&0022 != 0 }
