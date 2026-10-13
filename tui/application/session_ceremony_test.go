package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type journalStub struct {
	entries []TurnEntry
	err     error
}

func (j *journalStub) RecordTurn(_ context.Context, entry TurnEntry) (string, error) {
	j.entries = append(j.entries, entry)
	return "ws:x:entry:turn:1", j.err
}

func sessionCeremonyFor(t *testing.T, engine *fakeEngine) (*SessionCeremony, *memoryStore, *journalStub, []string) {
	t.Helper()
	var logged []string
	store := &memoryStore{}
	journal := &journalStub{}
	labels := &contextLabels{labels: map[domain.SessionID]domain.SessionLabel{}}
	c := &SessionCeremony{Engine: engine, Journal: journal, Labels: labels, Store: store, Now: func() time.Time { return time.Date(2026, 10, 13, 16, 0, 0, 0, time.UTC) }, Log: func(line string) { logged = append(logged, line) }}
	return c, store, journal, logged
}

// A session opens its axlr_session instance at the first prompt, claims
// the turn step for each prompt, completes it with the turn's record and
// writes the results to KMP under ws:<session>.
func TestSessionCeremonyClaimsAndCompletesEachTurn(t *testing.T) {
	engine := &fakeEngine{}
	c, _, journal, _ := sessionCeremonyFor(t, engine)
	s := recordedTurn(t)
	first := s.Export()
	// The claim happens when the prompt is appended (index 0 here).
	if err := s.BeginTurn("segundo prompt", turnTools()); err != nil {
		t.Fatal(err)
	}
	c.BeginTurn(context.Background(), &s)
	instance, has := s.Instance()
	if !has || instance.Instance != "axlr-session-"+string(first.ID) || instance.Fence != "fence-turn" || instance.TurnIndex != len(s.Messages())-1 {
		t.Fatalf("instance %+v", instance)
	}
	if strings.Join(engine.calls, ";") != "start axlr_session 1.0 about=;claim turn "+itoa(len(s.Messages())-1) {
		t.Fatalf("calls %v", engine.calls)
	}
	if err := s.CompleteAssistant(assistant("listo")); err != nil {
		t.Fatal(err)
	}
	if err := c.EndTurn(context.Background(), &s); err != nil {
		t.Fatal(err)
	}
	instance, _ = s.Instance()
	if instance.Fence != "" || instance.Turns != 1 || !strings.Contains(instance.LastRecord, `"turn":1`) {
		t.Fatalf("after the turn %+v", instance)
	}
	if len(engine.completed) != 1 || engine.completed[0]["answer"] != "listo" || engine.completed[0]["prompt"] != "segundo prompt" {
		t.Fatalf("completed %+v", engine.completed)
	}
	// An answer from reading leaves nothing for KMP.
	if len(journal.entries) != 0 {
		t.Fatalf("journal %+v", journal.entries)
	}
}

func TestSessionCeremonyWritesTheResultsOfATurnToKMP(t *testing.T) {
	engine := &fakeEngine{}
	c, store, journal, _ := sessionCeremonyFor(t, engine)
	s := turnSession(t)
	if err := s.BeginTurn("añade hola.txt y comprueba", turnTools()); err != nil {
		t.Fatal(err)
	}
	c.BeginTurn(context.Background(), &s)
	recordTurnOn(t, &s)
	if err := c.EndTurn(context.Background(), &s); err != nil {
		t.Fatal(err)
	}
	if len(journal.entries) != 1 {
		t.Fatalf("journal %+v", journal.entries)
	}
	id := string(s.ID())
	entry := journal.entries[0]
	if entry.About != "ws:"+id || entry.Key != "axlr:turn:"+id+":0" || !strings.Contains(entry.Summary, "1 file written") || entry.Evidence[0] != "MADE axlr-session-"+id+" step turn #1" || entry.Labels["session"][0] != id {
		t.Fatalf("entry %+v", entry)
	}
	if len(store.states) == 0 {
		t.Fatal("EndTurn saves the session")
	}
	if engine.completed[0]["answer_kind"] != "changed" || len(engine.completed[0]["actions"].([]any)) != 4 {
		t.Fatalf("completed %+v", engine.completed[0])
	}
}

func TestSessionCeremonyCompletesAnInterruptedTurnBeforeTheNextClaim(t *testing.T) {
	engine := &fakeEngine{}
	c, _, _, _ := sessionCeremonyFor(t, engine)
	s := turnSession(t)
	if err := s.BeginTurn("primero", turnTools()); err != nil {
		t.Fatal(err)
	}
	c.BeginTurn(context.Background(), &s)
	// The console stopped: the fence stays in the sidecar, no EndTurn.
	if err := s.InterruptDraft(""); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn("segundo", turnTools()); err != nil {
		t.Fatal(err)
	}
	c.BeginTurn(context.Background(), &s)
	if strings.Join(engine.calls, ";") != "start axlr_session 1.0 about=;claim turn 0;complete turn;claim turn 1" {
		t.Fatalf("calls %v", engine.calls)
	}
	if engine.completed[0]["interrupted"] != true || engine.completed[0]["answer_kind"] != "paused" {
		t.Fatalf("interrupted record %+v", engine.completed[0])
	}
	instance, _ := s.Instance()
	if instance.Turns != 1 || instance.Fence != "fence-turn" || instance.TurnIndex != 1 {
		t.Fatalf("instance %+v", instance)
	}
}

func TestSessionCeremonyCloseClaimsAClosingTurnAndTransitions(t *testing.T) {
	engine := &fakeEngine{}
	c, store, _, _ := sessionCeremonyFor(t, engine)
	s := turnSession(t)
	if err := s.BeginTurn("primero", turnTools()); err != nil {
		t.Fatal(err)
	}
	c.BeginTurn(context.Background(), &s)
	if err := s.CompleteAssistant(assistant("ok")); err != nil {
		t.Fatal(err)
	}
	if err := c.EndTurn(context.Background(), &s); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(context.Background(), &s, "exit"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(engine.calls, ";") != "start axlr_session 1.0 about=;claim turn 0;complete turn;claim turn 1;complete turn;transition closed" {
		t.Fatalf("calls %v", engine.calls)
	}
	closing := engine.completed[1]
	if closing["closed"] != true || closing["summary"].(map[string]any)["turns"] != 1 {
		t.Fatalf("closing %+v", closing)
	}
	instance, _ := s.Instance()
	if !instance.Closed || len(store.states) < 2 {
		t.Fatalf("instance %+v saves %d", instance, len(store.states))
	}
	// A closed instance is left alone.
	if err := c.Close(context.Background(), &s, "exit"); err != nil || len(engine.calls) != 6 {
		t.Fatalf("%v %v", err, engine.calls)
	}
}

func TestSessionCeremonySaysOnceWhenMADEIsNotPreparedAndSkipsWorkers(t *testing.T) {
	engine := &fakeEngine{ready: NotPreparedError(MissingDefinition{Name: SessionDefinition, Version: SessionDefinitionVersion})}
	var logged []string
	c, _, _, _ := sessionCeremonyFor(t, engine)
	c.Log = func(line string) { logged = append(logged, line) }
	s := turnSession(t)
	for _, prompt := range []string{"uno", "dos"} {
		if err := s.BeginTurn(root.Text(prompt), turnTools()); err != nil {
			t.Fatal(err)
		}
		c.BeginTurn(context.Background(), &s)
		if err := s.CompleteAssistant(assistant("ok")); err != nil {
			t.Fatal(err)
		}
		if err := c.EndTurn(context.Background(), &s); err != nil {
			t.Fatal(err)
		}
	}
	instance, has := s.Instance()
	if !has || instance.Instance != "" || !strings.Contains(instance.Unavailable, "not published") || len(engine.calls) != 0 {
		t.Fatalf("instance %+v calls %v", instance, engine.calls)
	}
	if len(logged) != 1 {
		t.Fatalf("said %d times: %v", len(logged), logged)
	}
	worker := turnSession(t)
	if err := worker.SetMode(domain.ModeTask); err != nil {
		t.Fatal(err)
	}
	if err := worker.BeginTurn("task", turnTools()); err != nil {
		t.Fatal(err)
	}
	engine.ready = nil
	c.BeginTurn(context.Background(), &worker)
	if _, has := worker.Instance(); has || len(engine.calls) != 0 {
		t.Fatal("a plan worker runs no session instance")
	}
	var none *SessionCeremony
	none.BeginTurn(context.Background(), &worker)
	if err := none.EndTurn(context.Background(), &worker); err != nil {
		t.Fatal(err)
	}
}

func TestSessionCeremonyKeepsTheFenceWhenACompletionFails(t *testing.T) {
	engine := &fakeEngine{failCompletes: 1}
	c, _, _, _ := sessionCeremonyFor(t, engine)
	s := turnSession(t)
	if err := s.BeginTurn("primero", turnTools()); err != nil {
		t.Fatal(err)
	}
	c.BeginTurn(context.Background(), &s)
	if err := s.CompleteAssistant(assistant("ok")); err != nil {
		t.Fatal(err)
	}
	if err := c.EndTurn(context.Background(), &s); err != nil {
		t.Fatal(err)
	}
	instance, _ := s.Instance()
	if instance.Fence != "fence-turn" || instance.Turns != 1 {
		t.Fatalf("instance %+v", instance)
	}
	if !errors.Is(context.Canceled, context.Canceled) {
		t.Fatal("sanity")
	}
}

func TestSuccessorNumberAndExhausted(t *testing.T) {
	if successorNumber("axlr-session-0123456789abcdef0123456789abcdef") != 1 || successorNumber("axlr-session-0123456789abcdef0123456789abcdef-3") != 3 || instanceID("abc", 2) != "axlr-session-abc-2" {
		t.Fatal("successor numbering")
	}
	if !exhausted(errors.New("refused: step repeat exhausted")) || exhausted(errors.New("connection reset")) {
		t.Fatal("exhausted")
	}
}
