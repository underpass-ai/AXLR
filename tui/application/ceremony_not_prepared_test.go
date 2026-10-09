package application

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestNotPreparedErrorNamesMissingDefinitions(t *testing.T) {
	err := NotPreparedError(MissingDefinition{Name: "axlr_improve", Version: "1.0"})

	msg := err.Error()
	for _, want := range []string{
		"axlr_improve 1.0",
		"one-time",
		"open /mcp",
		"select MADE",
		"press p",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q does not contain %q", msg, want)
		}
	}
	// Where the prompt went is the console's to say: a plan task and a tool
	// result carry this text too, and the console may keep the prompt as a
	// "Not sent" row rather than in the composer.
	if strings.Contains(msg, "composer") {
		t.Fatalf("error %q claims where the prompt went", msg)
	}

	if !errors.Is(err, ErrCeremonyNotPrepared) {
		t.Fatalf("errors.Is(err, ErrCeremonyNotPrepared) = false for %q", msg)
	}
	wrapped := fmt.Errorf("begin ceremony: %w", err)
	if !errors.Is(wrapped, ErrCeremonyNotPrepared) {
		t.Fatalf("wrapped error lost ErrCeremonyNotPrepared: %q", wrapped)
	}

	multi := NotPreparedError(
		MissingDefinition{Name: "axlr_improve", Version: "1.0"},
		MissingDefinition{Name: "axlr_repair", Version: "1.0"},
	).Error()
	if !strings.Contains(multi, "axlr_improve 1.0") || !strings.Contains(multi, "axlr_repair 1.0") {
		t.Fatalf("multi-definition error does not name both: %q", multi)
	}
}
