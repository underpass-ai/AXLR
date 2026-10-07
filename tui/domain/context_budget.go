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
// context window. An unknown window, or one large enough for the default,
// keeps the default. The four limits stay in their default proportions.
func ContextBudgetForWindow(window ContextWindow) ContextBudget {
	budget := DefaultContextBudget()
	if window.Tokens() < MinimumContextWindow {
		return budget
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
