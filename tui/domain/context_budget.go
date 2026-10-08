package domain

import "errors"

// ContextBudget bounds the serialized messages sent to a provider. It does not
// change the durable transcript. Its limits are bytes, including JSON escaping.
type ContextBudget struct {
	maximum, lowWater, toolResult, checkpoint int
}

func NewContextBudget(maximum, lowWater, toolResult, checkpoint int) (ContextBudget, error) {
	if maximum <= 0 || lowWater <= 0 || lowWater >= maximum || toolResult <= 0 || checkpoint <= 0 || checkpoint >= lowWater || toolResult >= maximum {
		return ContextBudget{}, errors.New("invalid context byte budget")
	}
	return ContextBudget{maximum, lowWater, toolResult, checkpoint}, nil
}

// DefaultContextBudget is the ceiling: what a model whose known window holds
// it may receive. A model with an unknown window gets ContextBudgetForPrompt
// instead, since every byte it receives is paid for.
func DefaultContextBudget() ContextBudget {
	budget, _ := NewContextBudget(1024*1024, 768*1024, 64*1024, 16*1024)
	return budget
}

func (b ContextBudget) MaximumBytes() int    { return b.maximum }
func (b ContextBudget) LowWaterBytes() int   { return b.lowWater }
func (b ContextBudget) ToolResultBytes() int { return b.toolResult }
func (b ContextBudget) CheckpointBytes() int { return b.checkpoint }
func (b ContextBudget) Validate() error {
	_, err := NewContextBudget(b.maximum, b.lowWater, b.toolResult, b.checkpoint)
	return err
}

// CompactContextBudget is the small-model profile's budget: an 80 KiB
// ceiling (about 23K tokens at 3.5 bytes per token, leaving room in a 32K
// window), a 56 KiB low watermark, 8 KiB per tool result and a 4 KiB
// checkpoint.
func CompactContextBudget() ContextBudget {
	budget, _ := NewContextBudget(80<<10, 56<<10, 8<<10, 4<<10)
	return budget
}

// Smaller keeps, limit by limit, the smaller of two budgets.
func (b ContextBudget) Smaller(other ContextBudget) ContextBudget {
	smaller, err := NewContextBudget(min(b.maximum, other.maximum), min(b.lowWater, other.lowWater), min(b.toolResult, other.toolResult), min(b.checkpoint, other.checkpoint))
	if err != nil {
		return b
	}
	return smaller
}

// Projection for a known context window: the messages may use at most three
// quarters of the window, leaving the rest for the fixed prefix (guidance and
// tool schemas) and the reply. Bytes per token is deliberately conservative:
// Qwen-class tokenizers measured 3.6 bytes per token on Go source and 3.8 on
// Markdown, so 3 under-fills rather than overflows.
const (
	windowShareNumerator   = 3
	windowShareDenominator = 4
	windowBytesPerToken    = 3
	// MinimumContextWindow is the smallest window a budget can be derived from.
	MinimumContextWindow = 4096
)

// ContextBudgetForWindow scales the default byte budget down to a model's
// context window. A window large enough for the default keeps the default;
// an unknown window gets the default prompt budget, since nothing says the
// model holds more. The four limits stay in their default proportions.
func ContextBudgetForWindow(window ContextWindow) ContextBudget {
	budget := DefaultContextBudget()
	if window.Tokens() < MinimumContextWindow {
		return ContextBudgetForPrompt(DefaultPromptTokens)
	}
	maximum := window.Tokens() / windowShareDenominator * windowShareNumerator * windowBytesPerToken
	if maximum >= budget.maximum {
		return budget
	}
	lowWater := maximum / 4 * 3
	toolResult := min(budget.toolResult, maximum/8)
	checkpoint := min(budget.checkpoint, lowWater/8)
	scaled, err := NewContextBudget(maximum, lowWater, toolResult, checkpoint)
	if err != nil {
		return budget
	}
	return scaled
}

// Prompt budget for a model whose window is unknown: a remote model, whose
// every prompt token is paid for. The limits are derived from the prompt size
// the console is willing to send, not from a window.
const (
	// DefaultPromptTokens is the prompt a request may reach by default. It
	// stays under the 100K tokens above which Claude Haiku 5.5 costs five
	// times as much, with room for the guidance, the schemas and the reply.
	DefaultPromptTokens = 64000
	// MinimumPromptTokens is the smallest prompt a budget can be derived
	// from: below it the guidance and the schemas leave no room for a turn.
	MinimumPromptTokens = 16384
	// promptBytesPerTokenHundredths is what a prompt token held on Claude
	// Haiku 5.5 through OpenRouter: 2.44 bytes over the 198 requests of one
	// session (8 October 2026), against the 3 the window budget assumes.
	// Tokenizers that hold more bytes per token cost fewer tokens for the
	// same bytes, so the budget errs on the cheap side for them.
	promptBytesPerTokenHundredths = 244
	// promptPrefixReserve is what the guidance and the tool schemas, which
	// the projection does not bound, took at most in that session: 7.7 KB
	// and 9.7 KB.
	promptPrefixReserve = 17 << 10
)

// ContextBudgetForPrompt bounds the projection so that a request stays near
// tokens: the history gets the prompt's bytes minus the reserve for guidance
// and schemas, the low watermark is half the ceiling so a cut leaves room for
// many appends before the next one, a tool result is at most an eighth of the
// ceiling and the checkpoint at most a sixth of the low watermark: it maps
// every omitted turn (exact request, files written, memory recorded), and
// at an eighth the 50 turns of session c33e8e86 left no room for the map. A
// prompt below the minimum gets the default; one that holds the default
// ceiling or more keeps the default.
func ContextBudgetForPrompt(tokens int) ContextBudget {
	if tokens < MinimumPromptTokens {
		tokens = DefaultPromptTokens
	}
	ceiling := DefaultContextBudget()
	maximum := tokens*promptBytesPerTokenHundredths/100 - promptPrefixReserve
	if maximum >= ceiling.maximum {
		return ceiling
	}
	lowWater := maximum / 2
	toolResult := min(ceiling.toolResult, maximum/8)
	checkpoint := min(ceiling.checkpoint, lowWater/6)
	budget, err := NewContextBudget(maximum, lowWater, toolResult, checkpoint)
	if err != nil {
		return ceiling
	}
	return budget
}
