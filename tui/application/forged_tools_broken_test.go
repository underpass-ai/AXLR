package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// axlr_tools marks a forged tool whose files changed or went missing as
// broken, with the reason and a hint, before the model calls it.
func TestAxlrToolsMarksBrokenForgedTools(t *testing.T) {
	s := turnSession(t)
	forged := &forgedMemory{}
	host := HostToolUseCase{Forge: forged}
	if outcome, _ := host.Execute(context.Background(), s, hostIdentity(t, domain.HostOperationForgeTool), object(t, forgeArguments)); outcome.IsError {
		t.Fatalf("forge: %s", outcome.Content)
	}
	tools := hostIdentity(t, domain.HostOperationTools)
	list := func(args string) string {
		t.Helper()
		outcome, err := host.Execute(context.Background(), s, tools, object(t, args))
		if err != nil || outcome.IsError {
			t.Fatalf("axlr_tools %s: %s %v", args, outcome.Content, err)
		}
		return string(outcome.Content)
	}
	const reason = `forged tool \"word_count\" changed since it was forged`

	forged.verifyErr = errors.New(`forged tool "word_count" changed since it was forged`)
	search := list(`{"query":"words"}`)
	if !strings.Contains(search, `"broken":"`+reason+`"`) || !strings.Contains(search, "axlr_remove_tool") {
		t.Fatalf("search does not mark the broken tool: %s", search)
	}
	exact := list(`{"name":"word_count"}`)
	if !strings.Contains(exact, `"broken":"`+reason+`"`) || !strings.Contains(exact, "forge it again") {
		t.Fatalf("exact name does not mark the broken tool: %s", exact)
	}

	forged.verifyErr = nil
	intact := list(`{"name":"word_count"}`)
	if strings.Contains(intact, "broken") {
		t.Fatalf("intact tool marked broken: %s", intact)
	}
}
