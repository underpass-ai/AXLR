//go:build !windows

package storage

import "syscall"

func makeFIFO(path string) error { return syscall.Mkfifo(path, 0600) }
