package local

import (
	"bytes"
	"sync"
)

type capture struct {
	mu             sync.Mutex
	budget         int
	stdout, stderr bytes.Buffer
	captured       int
	discarded      int64
}
