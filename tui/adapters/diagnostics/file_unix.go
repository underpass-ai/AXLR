//go:build !windows

package diagnostics

import (
	"os"
	"syscall"
)

func privatePayloadDirectory(info os.FileInfo) bool {
	return info.IsDir() && info.Mode().Perm()&0077 == 0
}

func openDiagnosticFile(path string, flags int, perm os.FileMode) (*os.File, error) {
	fd, err := syscall.Open(path, flags|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, uint32(perm))
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}
