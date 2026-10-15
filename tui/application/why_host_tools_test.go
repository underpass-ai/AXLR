package application

import (
	"context"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// On 16 October 2026 claude-haiku-5.5, told to give its writes and commands
// a why, gave axlr_forge_tool one too, and the strict decoding refused every
// registration, a one-line placeholder included. The console's own tools
// accept a why and drop it; an unknown field is still refused.
func TestHostToolsAcceptAndDropAWhy(t *testing.T) {
	forged := &forgedMemory{}
	s := turnSession(t)
	host := HostToolUseCase{Forge: forged}
	withWhy := strings.TrimSuffix(forgeArguments, "}") + `,"why":"Register the word counter."}`
	outcome, err := host.Execute(context.Background(), s, hostIdentity(t, domain.HostOperationForgeTool), object(t, withWhy))
	if err != nil || outcome.IsError || len(forged.tools) != 1 {
		t.Fatalf("forge with why = %s, %v", outcome.Content, err)
	}
	outcome, err = host.Execute(context.Background(), s, hostIdentity(t, domain.HostOperationRemoveTool), object(t, `{"name":"word_count","why":"Drop the counter."}`))
	if err != nil || outcome.IsError || len(forged.tools) != 0 {
		t.Fatalf("remove with why = %s, %v", outcome.Content, err)
	}
	other := strings.TrimSuffix(forgeArguments, "}") + `,"reason":"Register the word counter."}`
	if outcome, _ := host.Execute(context.Background(), s, hostIdentity(t, domain.HostOperationForgeTool), object(t, other)); !outcome.IsError {
		t.Fatalf("forge with an unknown field succeeded: %s", outcome.Content)
	}
}
