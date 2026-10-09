package terminal

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// pricedStream answers every request with text and a usage that costs cost.
type pricedStream struct {
	requests *int
	cost     float64
}

func (s pricedStream) Stream(_ context.Context, _ root.CompletionRequest, emit func(root.Text) error) (root.CompletionResult, error) {
	*s.requests++
	_ = emit("answer")
	return root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "answer"}, Provider: "Relace", Usage: &root.TokenUsage{PromptTokens: 1000, CompletionTokens: 100, TotalTokens: 1100, Cost: s.cost, CostKnown: true}}, nil
}

func sendPrompt(t *testing.T, m AppModel, prompt string) AppModel {
	t.Helper()
	m.Composer.Input.SetValue(prompt)
	next, cmd := m.Update(ControlIntent("send"))
	return drain(t, next.(AppModel), cmd)
}

func footerText(m AppModel) string { return ansi.Strip(m.footerStatus()) }

// The footer adds the turn's and the session's cost; past 80 % of
// max_session_usd the person is told once, at the limit the next request is
// refused with how to go on, /usage shows the account and + allows another
// max_session_usd, after which Ctrl+R goes on. A new console reads the
// session's cost back.
func TestCostInTheFooterAndTheSessionBudget(t *testing.T) {
	s := navSession(t)
	m := navModel(t, &s)
	store := m.deps.Store.(*storage.SessionStore)
	m.deps.Usage, m.deps.MaxSessionUSD = store, 0.5
	requests := 0
	continuation := application.ContinueTurnUseCase{Store: store, Models: pricedStream{&requests, 0.3}, Usage: store, MaxSessionUSD: 0.5}
	m.deps.Start = application.StartTurnUseCase{Catalog: readToolCatalog{}, Store: store, Continue: continuation}
	m.deps.Agent = application.AgentTurnUseCase{Continue: continuation}

	m = sendPrompt(t, m, "first")
	if got := footerText(m); !strings.HasSuffix(got, "1.1k tok · $0.30 turn · $0.30 session") || m.Status.Notice != "" {
		t.Fatalf("after the first request: %q, notice %q", got, m.Status.Notice)
	}
	m = sendPrompt(t, m, "second")
	if want := Translatef(English, "notice.budgetWarning", "$0.60", "$0.50"); m.Status.Notice != want {
		t.Fatalf("notice %q; want %q", m.Status.Notice, want)
	}
	if got := footerText(m); !strings.Contains(got, "$0.30 turn · $0.60 session") {
		t.Fatalf("after the second request: %q", got)
	}
	m = sendPrompt(t, m, "third")
	if want := Translatef(English, "error.sessionBudget", "$0.50", "$0.50"); m.Status.Error != want || requests != 2 || s.Status() != domain.StatusInterrupted {
		t.Fatalf("refusal: %q (want %q), requests %d, status %s", m.Status.Error, want, requests, s.Status())
	}
	if got := footerText(m); strings.Contains(got, "turn") || !strings.HasSuffix(got, "$0.60 session") {
		t.Fatalf("a refused turn has no cost of its own: %q", got)
	}

	m.Composer.Input.SetValue("/uso")
	next, _ := m.Update(ControlIntent("send"))
	m = next.(AppModel)
	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"Usage and cost", "+ allows another $0.50", "Requests: 2, 2 with a reported cost", "Relace · 2 requests · $0.60", "Budget: $0.50 · $0.00 left", "The budget is spent: + allows another $0.50"} {
		if m.overlay != "usage" || !strings.Contains(view, want) {
			t.Fatalf("/usage lacks %q:\n%s", want, view)
		}
	}
	m = update(m, tea.KeyPressMsg{Code: '+', Text: "+"})
	if ledger, err := store.LoadUsage(context.Background(), s.Export().ID); err != nil || ledger.Raised != 0.5 || m.Status.Notice != Translatef(English, "usage.raised", "$1.00") {
		t.Fatalf("raise: %+v %v, notice %q", ledger, err, m.Status.Notice)
	}
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, "Budget: $1.00, $0.50 of it raised for this session · $0.40 left") || strings.Contains(view, "allows another") {
		t.Fatalf("after the raise:\n%s", view)
	}
	// Below 80 % of the raised limit, + changes nothing.
	m = update(m, tea.KeyPressMsg{Code: '+', Text: "+"})
	if ledger, _ := store.LoadUsage(context.Background(), s.Export().ID); ledger.Raised != 0.5 {
		t.Fatalf("raised again below the warning: %+v", ledger)
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m = drain(t, next.(AppModel), cmd)
	if got := footerText(m); requests != 3 || m.overlay != "" || !strings.Contains(got, "$0.30 turn · $0.90 session") {
		t.Fatalf("ctrl+r after the raise: requests %d, overlay %q, footer %q", requests, m.overlay, got)
	}

	restarted := update(New(Dependencies{Session: &s, Store: store, Usage: store, MaxSessionUSD: 0.5, Monochrome: true}), tea.WindowSizeMsg{Width: 120, Height: 20})
	if got := footerText(restarted); strings.Contains(got, "turn") || !strings.HasSuffix(got, "$0.90 session") {
		t.Fatalf("after a restart: %q", got)
	}
}

// Under a cent the cost keeps four decimals, and a session whose requests
// reported no cost, a local model's, shows none rather than $0.
func TestFooterCostFormatAndLanguage(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m.cost = costView{ledger: domain.UsageLedger{Requests: 9, Cost: 0.2131, CostRequests: 8}, turnCost: 0.21, turnRequests: 7}
	if got := footerText(m); !strings.HasSuffix(got, "idle · $0.0031 turn · $0.21 session") {
		t.Fatalf("english: %q", got)
	}
	m.Theme.Locale = Spanish
	if got := footerText(m); !strings.HasSuffix(got, "$0.0031 turno · $0.21 sesión") {
		t.Fatalf("spanish: %q", got)
	}
	m.cost = costView{ledger: domain.UsageLedger{Requests: 4}}
	m.tokens = 12_400
	if got := footerText(m); strings.Contains(got, "$") || !strings.HasSuffix(got, "12.4k tok") {
		t.Fatalf("unknown cost shown: %q", got)
	}
}

// The panel lists providers by cost, says "—" for one without a cost and
// for an unmeasured latency, and speaks Spanish.
func TestUsagePanelContent(t *testing.T) {
	var l domain.UsageLedger
	l.Record(domain.UsageSample{Provider: "local/qwen3.8-27b", PromptTokens: 64_000, CompletionTokens: 900, Total: 40 * time.Second}, 1)
	l.Record(domain.UsageSample{Provider: "InferenceNet", PromptTokens: 7448, CachedTokens: 6000, CacheWriteTokens: 1000, CompletionTokens: 2567, ReasoningTokens: 2590, Cost: 0.0016, CostKnown: true, FirstByte: 1100 * time.Millisecond, Total: 9 * time.Second}, 1)
	l.Record(domain.UsageSample{Provider: "Relace", PromptTokens: 7000, CompletionTokens: 500, Cost: 0.0042, CostKnown: true, FirstByte: 800 * time.Millisecond, Total: 2 * time.Second}, 1)
	got := usageContent(l, 1, Theme{Locale: English})
	want := strings.Join([]string{
		"Requests: 3, 2 with a reported cost",
		"Prompt tokens: 78.4k · read from cache 6.0k · written to cache 1.0k",
		"Completion tokens: 4.0k · reasoning 2.6k",
		"",
		"Cost: $0.0058",
		"By provider",
		"  Relace · 1 requests · $0.0042",
		"  InferenceNet · 1 requests · $0.0016",
		"  local/qwen3.8-27b · 1 requests · —",
		"",
		"First byte: average 950ms · max 1.1s",
		"Whole request: average 17.0s · max 40.0s",
		"",
		"Budget: $1.00 · $0.99 left",
	}, "\n")
	if got != want {
		t.Fatalf("panel:\n%s\nwant:\n%s", got, want)
	}
	spanish := usageContent(domain.UsageLedger{Requests: 1, Total: domain.LatencyStats{Count: 1, Sum: time.Second, Max: time.Second}}, 0, Theme{Locale: Spanish})
	for _, want := range []string{"Peticiones: 1, 0 con coste informado", "Coste: desconocido", "Primer byte: media — · máximo —", "Presupuesto: ninguno (max_session_usd es 0)"} {
		if !strings.Contains(spanish, want) {
			t.Fatalf("spanish panel lacks %q:\n%s", want, spanish)
		}
	}
	if empty := usageContent(domain.UsageLedger{}, 2, Theme{Locale: English}); !strings.HasPrefix(empty, "No model request recorded") || !strings.HasSuffix(empty, "Budget: $2.00 · $2.00 left") {
		t.Fatalf("empty panel: %q", empty)
	}
}

// The action palette opens the panel too.
func TestThePaletteOpensUsage(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m = update(m, ControlIntent("palette"))
	m = update(m, tea.KeyPressMsg{Code: '$', Text: "$"})
	if m.overlay != "usage" {
		t.Fatalf("overlay %q", m.overlay)
	}
}
