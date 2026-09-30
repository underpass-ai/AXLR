//go:build windows

package storage

import "errors"

func makeFIFO(string) error { return errors.New("POSIX FIFO unavailable") }
