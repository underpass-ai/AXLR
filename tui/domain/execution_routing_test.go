package domain

import (
	"math"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
)

func TestExecutionRoutingValidates(t *testing.T) {
	models := func(ids ...string) []root.ModelID {
		out := make([]root.ModelID, 0, len(ids))
		for _, id := range ids {
			out = append(out, root.ModelID(id))
		}
		return out
	}
	if _, err := NewExecutionRouting(models("local", "anthropic/claude-haiku-5.5"), RoutingCost, false, 32000, 0.05); err != nil {
		t.Fatalf("a valid routing was refused: %v", err)
	}
	for name, build := range map[string]func() error{
		"no candidate":    func() error { _, err := NewExecutionRouting(nil, RoutingCost, true, 0, 0); return err },
		"duplicate":       func() error { _, err := NewExecutionRouting(models("a", "a"), RoutingCost, true, 0, 0); return err },
		"blank candidate": func() error { _, err := NewExecutionRouting(models("a", " "), RoutingCost, true, 0, 0); return err },
		"too many": func() error {
			_, err := NewExecutionRouting(models("1", "2", "3", "4", "5", "6", "7", "8", "9"), RoutingCost, true, 0, 0)
			return err
		},
		"objective":        func() error { _, err := NewExecutionRouting(models("a"), "cheapest", true, 0, 0); return err },
		"negative context": func() error { _, err := NewExecutionRouting(models("a"), RoutingCost, true, -1, 0); return err },
		"huge context": func() error {
			_, err := NewExecutionRouting(models("a"), RoutingCost, true, MaxRoutingContextTokens+1, 0)
			return err
		},
		"negative ceiling":   func() error { _, err := NewExecutionRouting(models("a"), RoutingCost, true, 0, -0.01); return err },
		"not-a-number limit": func() error { _, err := NewExecutionRouting(models("a"), RoutingCost, true, 0, math.NaN()); return err },
	} {
		if err := build(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestExecutionRoutingCapNarrowsTheModelsCap(t *testing.T) {
	routing, _ := NewExecutionRouting([]root.ModelID{"a"}, RoutingCost, true, 32000, 0)
	if got := routing.Cap(48000); got != 32000 {
		t.Fatalf("cap = %d, want max_context_tokens 32000", got)
	}
	if got := routing.Cap(24000); got != 24000 {
		t.Fatalf("cap = %d, want the model's own 24000", got)
	}
	if got := routing.Cap(0); got != 32000 {
		t.Fatalf("cap of an unknown window = %d, want 32000", got)
	}
	open, _ := NewExecutionRouting([]root.ModelID{"a"}, RoutingCost, true, 0, 0)
	if got := open.Cap(24000); got != 24000 {
		t.Fatalf("cap without max_context_tokens = %d", got)
	}
}

func TestSessionKeepsItsRoutingThroughRestore(t *testing.T) {
	session, err := NewSession("0123456789abcdef0123456789abcdef", Workspace(t.TempDir()), "local")
	if err != nil {
		t.Fatal(err)
	}
	routing, _ := NewExecutionRouting([]root.ModelID{"local", "remote"}, RoutingLatency, false, 0, 0.5)
	if err := session.SetRouting(routing); err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreSession(session.Export())
	if err != nil {
		t.Fatal(err)
	}
	got, has := restored.Routing()
	if !has || got.Objective() != RoutingLatency || got.AllowsRemote() || got.MaxRequestUSD() != 0.5 || strings.Join(modelStrings(got.Candidates()), ",") != "local,remote" {
		t.Fatalf("restored routing = %+v, %v", got, has)
	}
	candidates := got.Candidates()
	candidates[0] = "changed"
	if again, _ := restored.Routing(); again.Candidates()[0] != "local" {
		t.Fatal("a caller changed the session's candidates")
	}
}

func modelStrings(models []root.ModelID) []string {
	out := make([]string, 0, len(models))
	for _, model := range models {
		out = append(out, string(model))
	}
	return out
}
