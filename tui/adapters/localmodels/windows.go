package localmodels

import (
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// Windows answers what the console knows about a model's context: the local
// model's configured window, capped by settings' context_tokens; a remote
// model has no known window here, so only the cap applies to it, and its
// budget comes from settings' prompt_tokens instead.
type Windows struct {
	Local map[root.ModelID]domain.ContextWindow
	Cap   domain.ContextWindow
	// Prompt is the prompt budget, in tokens, for a model whose window is
	// unknown; zero means domain.DefaultPromptTokens.
	Prompt int
}

func (w Windows) ContextWindow(id root.ModelID) domain.ContextWindow {
	window := w.Local[id]
	if w.Cap > 0 && (window <= 0 || w.Cap < window) {
		window = w.Cap
	}
	return window
}

// ContextBudget derives a local model's budget from its window, which its
// server holds for free, and a remote model's from the prompt budget, since
// every token it receives is paid for; a cap still bounds the remote one.
func (w Windows) ContextBudget(id root.ModelID) domain.ContextBudget {
	if _, local := w.Local[id]; local {
		return domain.ContextBudgetForWindow(w.ContextWindow(id))
	}
	budget := domain.ContextBudgetForPrompt(w.Prompt)
	if w.Cap > 0 {
		budget = budget.Smaller(domain.ContextBudgetForWindow(w.Cap))
	}
	return budget
}
