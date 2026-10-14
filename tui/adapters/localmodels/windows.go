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
	// Calibration is what a remote model's prompt tokens measured, in
	// hundredths of a byte per token; nil, or a model without enough
	// measured requests, keeps the ratio the prompt budget assumes.
	Calibration interface {
		BytesPerToken(root.ModelID) (int, bool)
	}
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
// every token it receives is paid for, at the bytes per token the model was
// measured to hold once it was; a cap still bounds the remote one.
func (w Windows) ContextBudget(id root.ModelID) domain.ContextBudget {
	if _, local := w.Local[id]; local {
		return domain.ContextBudgetForWindow(w.ContextWindow(id))
	}
	budget := domain.ContextBudgetForPrompt(w.Prompt)
	if w.Calibration != nil {
		if hundredths, ok := w.Calibration.BytesPerToken(id); ok {
			budget = domain.ContextBudgetForPromptAt(w.Prompt, hundredths)
		}
	}
	if w.Cap > 0 {
		budget = budget.Smaller(domain.ContextBudgetForWindow(w.Cap))
	}
	return budget
}

// PromptCeiling is the prompt, in tokens, the console lets a request to the
// model reach, for the footer's "27k/64k": a local model's window, and a
// remote model's prompt budget (prompt_tokens, by default
// domain.DefaultPromptTokens), within the cap.
func (w Windows) PromptCeiling(id root.ModelID) int {
	if _, local := w.Local[id]; local {
		return int(w.ContextWindow(id))
	}
	ceiling := w.Prompt
	if ceiling <= 0 {
		ceiling = domain.DefaultPromptTokens
	}
	if w.Cap > 0 && int(w.Cap) < ceiling {
		ceiling = int(w.Cap)
	}
	return ceiling
}

// ContextCap is the cap, in tokens, of a request's whole context for a
// model (decision D10): three quarters of a local model's window, leaving
// the rest for the reply; a remote model's prompt ceiling.
func (w Windows) ContextCap(id root.ModelID) int {
	if _, local := w.Local[id]; local {
		return int(w.ContextWindow(id)) / 4 * 3
	}
	return w.PromptCeiling(id)
}
