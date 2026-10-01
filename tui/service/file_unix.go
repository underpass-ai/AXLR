//go:build !windows

package service

import "os"

func syncDirectoryFile(file *os.File) error { return file.Sync() }

func secureStateDirectory(_ string) error { return nil }
