//go:build windows

package diagnostics

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func openDiagnosticFile(path string, flags int, _ os.FileMode) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	access := uint32(windows.GENERIC_WRITE)
	if flags&os.O_APPEND != 0 {
		access = windows.FILE_APPEND_DATA
	}
	create := uint32(windows.OPEN_EXISTING)
	if flags&os.O_CREATE != 0 {
		create = windows.OPEN_ALWAYS
	}
	if flags&os.O_EXCL != 0 {
		create = windows.CREATE_NEW
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
