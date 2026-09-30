//go:build windows

package service

import "os"

// Go's Windows directory handles cannot be flushed. Data files are synced
// before their atomic rename.
func syncDirectoryFile(_ *os.File) error { return nil }
