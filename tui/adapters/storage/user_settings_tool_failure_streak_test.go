package storage

import (
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// tool_failure_streak is 2 to 32, and absent it ends a run at the default.
func TestToolFailureStreakIsBoundedWithADefault(t *testing.T) {
	for _, value := range []int{0, 2, 3, 32} {
		settings := UserSettings{Language: "en", Theme: "auto", Icons: "safe", ToolFailureStreak: value}
		if err := settings.Validate(); err != nil {
			t.Fatalf("tool_failure_streak %d: %v", value, err)
		}
		want := value
		if value == 0 {
			want = domain.DefaultToolFailureStreak
		}
		if got := settings.ExecToolFailureStreak(); got != want {
			t.Fatalf("tool_failure_streak %d reads %d", value, got)
		}
	}
	for _, value := range []int{-1, 1, 33} {
		err := (UserSettings{Language: "en", Theme: "auto", Icons: "safe", ToolFailureStreak: value}).Validate()
		if err == nil || !strings.Contains(err.Error(), "tool_failure_streak must be between 2 and 32") {
			t.Fatalf("tool_failure_streak %d: %v", value, err)
		}
	}
}
