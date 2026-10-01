//go:build windows

package service

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// secureStateDirectory replaces inherited ACLs before any service state is
// opened. Existing contents are covered as well as files created later.
func secureStateDirectory(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + user.User.Sid.String() + ")(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)")
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return filepath.WalkDir(path, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		p, err := windows.UTF16PtrFromString(name)
		if err != nil {
			return err
		}
		attributes, err := windows.GetFileAttributes(p)
		if err != nil {
			return err
		}
		if attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return errors.New("reparse point in service state")
		}
		return windows.SetNamedSecurityInfo(name, windows.SE_FILE_OBJECT,
			windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
			nil, nil, dacl, nil)
	})
}

// Go's Windows directory handles cannot be flushed. Data files are synced
// before their atomic rename.
func syncDirectoryFile(_ *os.File) error { return nil }
