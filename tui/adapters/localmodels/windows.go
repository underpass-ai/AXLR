package localmodels

import (
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// Windows answers the context window the console respects for a model: the
// local model's configured window, capped by settings' context_tokens. Remote
// models have no known window here, so only the cap applies to them.
type Windows struct {
	Local map[root.ModelID]domain.ContextWindow
	Cap   domain.ContextWindow
}

func (w Windows) ContextWindow(id root.ModelID) domain.ContextWindow {
	window := w.Local[id]
	if w.Cap > 0 && (window <= 0 || w.Cap < window) {
		window = w.Cap
	}
	return window
}
