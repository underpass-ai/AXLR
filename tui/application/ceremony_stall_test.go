package application

import (
	"context"
	"strings"
	"testing"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func stalledDelivery(t *testing.T) (*fakeEngine, *CeremonyDriver, domain.Session) {
	t.Helper()
	engine := &fakeEngine{}
	d := &CeremonyDriver{Engine: engine, Checks: &fakeChecks{}, Now: func() time.Time { return time.Unix(1, 0) }}
	s := turnSession(t)
	if err := s.SetMode(domain.ModeDelivery); err != nil {
		t.Fatal(err)
	}
	if err := d.Begin(context.Background(), &s, "fix it"); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn("fix it", turnTools()); err != nil {
		t.Fatal(err)
	}
	return engine, d, s
}

func TestASecondReplyWithoutAToolCallCancelsTheCeremony(t *testing.T) {
	engine, d, s := stalledDelivery(t)
	var prompts []string
	calls := 0
	u := AgentTurnUseCase{Continue: ContinueTurnUseCase{Store: &memoryStore{}, Ceremonies: d, Models: streamFunc(func(_ context.Context, r root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		calls++
		prompts = append(prompts, string(r.Messages[len(r.Messages)-1].Content))
		return assistant(`<|tool_call>call:local_read{path:<|"|>x<|"|>}<tool_call|>`), nil
	})}}
	if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil {
		t.Fatal(err)
	}
	if _, live := s.Ceremony(); live || s.Mode() != domain.ModeNormal {
		t.Fatal("a stalled ceremony is still live")
	}
	if !strings.Contains(strings.Join(engine.calls, "|"), "cancel AXLR console: the model ended its turn twice") {
		t.Fatalf("not cancelled in MADE: %v", engine.calls)
	}
	// The reminder named the leaked call; the notice names the parser.
	if calls != 3 || !strings.Contains(prompts[1], "wrote a tool call as text (<|tool_call>)") || !strings.Contains(prompts[2], "tool-call parser is the first suspect") {
		t.Fatalf("calls=%d prompts=%q", calls, prompts)
	}
}

func TestTheReminderIsPlainWithoutLeakedMarkup(t *testing.T) {
	_, d, s := stalledDelivery(t)
	var second string
	calls := 0
	u := AgentTurnUseCase{Continue: ContinueTurnUseCase{Store: &memoryStore{}, Ceremonies: d, Models: streamFunc(func(_ context.Context, r root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		calls++
		if calls == 2 {
			second = string(r.Messages[len(r.Messages)-1].Content)
		}
		return assistant("I think the brief is clear."), nil
	})}}
	if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(second, "as text") || !strings.Contains(second, "still open") {
		t.Fatalf("reminder = %q", second)
	}
}

func TestAnOpenStepWithoutATurnCanBeStoppedByThePerson(t *testing.T) {
	engine, d, s := stalledDelivery(t)
	if err := s.CompleteAssistant(assistant("paused")); err != nil {
		t.Fatal(err)
	}
	if run, idle := OpenStepIdle(s); !idle || run.Step != "brief" {
		t.Fatalf("open step not reported: %v", idle)
	}
	store := &memoryStore{}
	if err := (StartTurnUseCase{Store: store, Continue: ContinueTurnUseCase{Ceremonies: d}}).StopCeremony(context.Background(), &s, ignoreEvent); err != nil {
		t.Fatal(err)
	}
	if _, live := s.Ceremony(); live || len(store.states) != 1 || !strings.Contains(strings.Join(engine.calls, "|"), "cancel AXLR console: stopped by the person") {
		t.Fatalf("stop: live=%v saves=%d calls=%v", live, len(store.states), engine.calls)
	}
	if _, idle := OpenStepIdle(s); idle {
		t.Fatal("a stopped ceremony is still reported open")
	}
}

func TestSelfRepairAndSelfImprovementKeepTheirOwnNudges(t *testing.T) {
	for mode, run := range map[domain.WorkMode]domain.CeremonyRun{
		domain.ModeRepair:  {Definition: "axlr_repair", Version: "1.0", Instance: "i", Step: "repair", Iteration: 1, Reminded: true},
		domain.ModeImprove: {Definition: "axlr_improve", Version: "1.0", Instance: "i", Step: "brief", Iteration: 1, Reminded: true, Repair: &domain.RepairRun{Improvement: true}},
	} {
		d := &CeremonyDriver{Engine: &fakeEngine{}, Checks: &fakeChecks{}}
		s := turnSession(t)
		if err := s.SetMode(mode); err != nil {
			t.Fatal(err)
		}
		if err := s.BeginTurn("work", turnTools()); err != nil {
			t.Fatal(err)
		}
		if err := s.SetCeremony(run); err != nil {
			t.Fatal(err)
		}
		if err := s.CompleteAssistant(assistant("done?")); err != nil {
			t.Fatal(err)
		}
		if StalledReason(s) == "" {
			t.Fatalf("%s: the case is not stalled", mode)
		}
		if cancelled, err := cancelStalled(context.Background(), &s, ContinueTurnUseCase{Store: &memoryStore{}, Ceremonies: d}, ignoreEvent); err != nil || cancelled {
			t.Fatalf("%s was cancelled: %v %v", mode, cancelled, err)
		}
		if _, live := s.Ceremony(); !live {
			t.Fatalf("the %s ceremony was ended", mode)
		}
	}
}
