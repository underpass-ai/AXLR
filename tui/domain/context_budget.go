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
	budget, _ := NewContextBudget(256*1024, 192*1024, 32*1024, 16*1024)
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
