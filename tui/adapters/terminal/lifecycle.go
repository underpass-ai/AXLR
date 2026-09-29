package terminal

import (
	"context"
	"sync"
)

// lifecycle is shared by Bubble Tea's model copies and the composition owner.
type lifecycle struct {
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	closed  bool
	workers sync.WaitGroup
}

func (m AppModel) Close() {
	if m.lifetime == nil {
		return
	}
	m.lifetime.mu.Lock()
	m.lifetime.closed = true
	m.lifetime.cancel()
	m.lifetime.mu.Unlock()
	m.lifetime.workers.Wait()
	m.zones.Close()
}
