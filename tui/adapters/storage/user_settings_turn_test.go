package storage

import (
	"strings"
	"testing"
)

func TestTurnToolCallsIsBounded(t *testing.T) {
	for _, value := range []int{0, 8, 32, 256} {
		if err := (UserSettings{Language: "en", Theme: "auto", Icons: "safe", TurnToolCalls: value}).Validate(); err != nil {
			t.Fatalf("turn_tool_calls %d: %v", value, err)
		}
	}
	for _, value := range []int{-1, 7, 257} {
		err := (UserSettings{Language: "en", Theme: "auto", Icons: "safe", TurnToolCalls: value}).Validate()
		if err == nil || !strings.Contains(err.Error(), "turn_tool_calls must be between 8 and 256") {
			t.Fatalf("turn_tool_calls %d: %v", value, err)
		}
	}
}
