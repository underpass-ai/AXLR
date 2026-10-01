//go:build windows

package storage

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func openNoFollow(path string, flags int, _ os.FileMode) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	access := uint32(windows.GENERIC_READ)
	if flags&os.O_RDWR != 0 {
		access = windows.GENERIC_READ | windows.GENERIC_WRITE
	} else if flags&os.O_WRONLY != 0 {
		access = windows.GENERIC_WRITE
	}
	create := uint32(windows.OPEN_EXISTING)
	if flags&os.O_CREATE != 0 {
		create = windows.OPEN_ALWAYS
		access |= windows.GENERIC_WRITE
	}
	handle, err := windows.CreateFile(p, access, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, create, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		windows.CloseHandle(handle)
		return nil, err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		windows.CloseHandle(handle)
		return nil, errors.New("reparse point is not allowed")
	}
	return os.NewFile(uintptr(handle), path), nil
}

func privateRegular(info os.FileInfo) bool { return info.Mode().IsRegular() }

func tryLock(file *os.File) error {
	return windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &windows.Overlapped{})
}

func lockBusy(err error) bool {
	return errors.Is(err, windows.ERROR_LOCK_VIOLATION) || errors.Is(err, windows.ERROR_SHARING_VIOLATION)
}

// Windows does not permit FlushFileBuffers on directory handles. File content
// is synced before rename; the rename itself uses the platform's atomic move.
func syncDirectoryFile(_ *os.File) error { return nil }

// Windows reports synthetic permission bits; access is governed by ACLs.
func writableByOthers(os.FileInfo) bool { return false }
