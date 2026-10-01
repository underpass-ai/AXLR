//go:build windows

package runtime

import "errors"

func makeFIFO(string) error { return errors.New("POSIX FIFO unavailable") }
