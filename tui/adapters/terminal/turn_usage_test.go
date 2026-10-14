package terminal

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/application"
)

// Each finished turn ends with a dim line of what it cost and the tokens
// it used, the footer reads the last prompt against the prompt ceiling,
// and /usage lists the last turns of this console.
func TestEachTurnShowsWhatItCost(t *testing.T) {
	s := navSession(t)
	m := navModel(t, &s)
	defer m.zones.Close()
	store := m.deps.Store.(*storage.SessionStore)
	m.deps.Usage = store
	m.deps.PromptCeiling = func(root.ModelID) int { return 64000 }
	requests := 0
	continuation := application.ContinueTurnUseCase{Store: store, Models: pricedStream{&requests, 0.0046}, Usage: store}
	m.deps.Start = application.StartTurnUseCase{Catalog: readToolCatalog{}, Store: store, Continue: continuation}
	m = sendPrompt(t, m, "first")
	m = sendPrompt(t, m, "second")
	text := m.Transcript.Text()
	if strings.Count(text, "· $0.0046 · 1.1k tok") != 2 {
		t.Fatalf("turn lines:\n%s", text)
	}
	if !strings.Contains(text, "answer\n  · $0.0046 · 1.1k tok\n\n> second") {
		t.Fatalf("the line is not under its reply:\n%s", text)
	}
	if got := footerText(m); !strings.Contains(got, "1k/64k tok") {
		t.Fatalf("footer %q", got)
	}
	m = m.openUsagePanel()
	view := ansi.Strip(m.Info.Text())
	if !strings.Contains(view, "Last turns in this console") || strings.Count(view, "· 1 request · 1.1k tok · $0.0046 · ") != 2 {
		t.Fatalf("/usage:\n%s", view)
	}
}

func TestTurnLines(t *testing.T) {
	if got := turnCostLine(turnUsage{Tokens: 27_400, Cost: 0.0046, CostKnown: true}); got != "· $0.0046 · 27k tok" {
		t.Fatalf("line %q", got)
	}
	if got := turnCostLine(turnUsage{Tokens: 900}); got != "· 900 tok" {
		t.Fatalf("no cost %q", got)
	}
	if got := turnCostLine(turnUsage{}); got != "" {
		t.Fatalf("nothing known %q", got)
	}
	for n, want := range map[int]string{980: "980", 9_500: "9.5k", 27_400: "27k", 64_000: "64k", 1_200_000: "1.2M", 2_000_000: "2M"} {
		if got := shortTokens(n); got != want {
			t.Fatalf("%d: %q", n, got)
		}
	}
	var m AppModel
	for i := 0; i < 12; i++ {
		m.cost.ledger.Requests++
		m.finishTurnUsage(i, time.Second)
		m.cost.beginTurn()
	}
	if len(m.turnLog) != maxTurnLog || m.turnLog[0].Requests != 1 {
		t.Fatalf("log %+v", m.turnLog)
	}
	m.finishTurnUsage(20, time.Second)
	if _, ok := m.turnCosts[20]; ok {
		t.Fatal("a turn without a request has a line")
	}
}

// On 14 October 2026 /usage listed a turn begun at 15:46 as "15:49 · 8
// requests", its end time, and nothing said which turn it was: it shows
// when the turn began and its prompt's first words.
func TestUsageTurnsShowTheirStartAndPrompt(t *testing.T) {
	var m AppModel
	m.Header.State.Messages = []root.Message{
		{Role: root.RoleUser, Content: "  arregla el bug de la calculadora que resta en vez de sumar\ny añade tests"},
		{Role: root.RoleUser, Content: "[AXLR · budget] note"},
		{Role: root.RoleAssistant, Content: "hecho"},
	}
	m.cost.ledger.Requests = 8
	m.finishTurnUsage(3, 3*time.Minute)
	turn := m.turnLog[0]
	if since := time.Since(turn.At); since < 3*time.Minute || since > 3*time.Minute+5*time.Second {
		t.Fatalf("the turn began %s ago", since)
	}
	if turn.Prompt != "arregla el bug de la…" {
		t.Fatalf("prompt %q", turn.Prompt)
	}
	lines := turnLogLines(m.turnLog, Theme{Monochrome: true})
	if len(lines) != 2 || !strings.Contains(lines[1], " · arregla el bug de la… · 8 requests") {
		t.Fatalf("lines %q", lines)
	}
}
