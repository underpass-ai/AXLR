package localmodels

import (
	"sync"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// ServedWindows holds the window each local model's server reported when it
// refused a request as too long. A launch learns it once and sizes every
// later request to it, instead of having each one refused first: on wrx80 on
// 20 November 2026 local_models.context_tokens said 200000 for a llama.cpp
// serving 90112, and 32 tool calls took 49 requests, one refused per step.
type ServedWindows struct {
	mu      sync.Mutex
	windows map[root.ModelID]domain.ContextWindow
}

func (s *ServedWindows) observe(id root.ModelID, window domain.ContextWindow) {
	if s == nil || window <= 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.windows == nil {
		s.windows = map[root.ModelID]domain.ContextWindow{}
	}
	if known, ok := s.windows[id]; !ok || window < known {
		s.windows[id] = window
	}
}

func (s *ServedWindows) window(id root.ModelID) (domain.ContextWindow, bool) {
	if s == nil {
		return 0, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	window, ok := s.windows[id]
	return window, ok
}
