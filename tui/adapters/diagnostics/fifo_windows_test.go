//go:build windows

package diagnostics_test

import "errors"

func makeFIFO(string) error { return errors.New("POSIX FIFO unavailable") }
