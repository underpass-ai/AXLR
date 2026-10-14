package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type journalStub struct {
	entries []TurnEntry
	settled []SettledEntry
	err     error
	// errFor answers one key with its own error.
	errFor map[string]error
}

func (j *journalStub) RecordTurn(_ context.Context, entry TurnEntry) (string, error) {
	j.entries = append(j.entries, entry)
	if err, ok := j.errFor[entry.Key]; ok {
		return "", err
	}
	return "ws:x:entry:turn:1", j.err
}

// An entry the store accepted already (its idempotency key conflicts) is
// dropped so the entries behind it are written; any other refusal keeps
// the queue from that entry on (seen on 13 October 2026: a close entry
// queued twice blocked a successor's turn records).
func TestJournalDropsAnEntryRecordedAlready(t *testing.T) {
	c, _, journal, _ := sessionCeremonyFor(t, &fakeEngine{})
	journal.errFor = map[string]error{"axlr:closed:axlr-session-x": fmt.Errorf("KMP refused the turn entry (conflict): %w", ErrEntryRecorded), "k3": errors.New("engine down")}
	var pending []string
	for _, key := range []string{"axlr:closed:axlr-session-x", "k2", "k3", "k4"} {
		pending = append(pending, encodeEntry(TurnEntry{Key: key, About: "ws:x", Summary: key}))
	}
	kept := c.journal(context.Background(), pending)
	if len(journal.entries) != 3 || journal.entries[1].Key != "k2" || journal.entries[2].Key != "k3" {
		t.Fatalf("written %+v", journal.entries)
	}
	if len(kept) != 2 {
		t.Fatalf("kept %d", len(kept))
	}
	if k3, _ := decodeEntry(kept[0]); k3.Key != "k3" {
		t.Fatalf("kept %+v", k3)
	}
}

// An entry KMP refuses for what it is is dropped: kept, it would block
// the queue behind it for good. One KMP could not take stays.
func TestJournalDropsAnEntryKMPRefusedForWhatItIs(t *testing.T) {
	c, _, journal, _ := sessionCeremonyFor(t, &fakeEngine{})
	journal.errFor = map[string]error{"k1": fmt.Errorf("KMP refused the turn entry (invalid_request): bad: %w", ErrEntryRefused), "k3": errors.New("io_error: disk full")}
	var pending []string
	for _, key := range []string{"k1", "k2", "k3", "k4"} {
		pending = append(pending, encodeEntry(TurnEntry{Key: key, About: "ws:x", Summary: key}))
	}
	kept := c.journal(context.Background(), pending)
	if len(journal.entries) != 3 || len(kept) != 2 {
		t.Fatalf("written %+v kept %d", journal.entries, len(kept))
	}
	if k3, _ := decodeEntry(kept[0]); k3.Key != "k3" {
		t.Fatalf("kept %+v", k3)
	}
}

func (j *journalStub) RecordSettled(_ context.Context, entry SettledEntry) (string, error) {
	j.settled = append(j.settled, entry)
	return entry.About + ":entry:" + entry.Kind + ":1", j.err
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
	if strings.Join(engine.calls, ";") != "start axlr_session 1.0 about=;claim turn "+itoa(len(s.Messages())-1)+";report running turn" {
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
	if entry.About != "ws:"+id || entry.Key != "axlr:turn:"+id+":0" || !strings.Contains(entry.Summary, "1 file written") || entry.Evidence[0] != "ceremony axlr-session-"+id+" step turn #1" || entry.Labels["session"][0] != id {
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
	if strings.Join(engine.calls, ";") != "start axlr_session 1.0 about=;claim turn 0;report running turn;report finished turn;complete turn;claim turn 1;report running turn" {
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
	if strings.Join(engine.calls, ";") != "start axlr_session 1.0 about=;claim turn 0;report running turn;report finished turn;complete turn;claim turn 1791907200;complete turn;transition closed" {
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
	if err := c.Close(context.Background(), &s, "exit"); err != nil || len(engine.calls) != 8 {
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
	// The turn is not counted until its completion lands: the next claim
	// completes it as interrupted and counts it then, once.
	if instance.Fence != "fence-turn" || instance.Turns != 0 {
		t.Fatalf("instance %+v", instance)
	}
	if err := s.BeginTurn("segundo", turnTools()); err != nil {
		t.Fatal(err)
	}
	c.BeginTurn(context.Background(), &s)
	instance, _ = s.Instance()
	if instance.Turns != 1 || len(engine.completed) != 1 || engine.completed[0]["turn"] != float64(1) || engine.completed[0]["interrupted"] != true {
		t.Fatalf("instance %+v completed %+v", instance, engine.completed)
	}
}

// The turn's record is saved even when the person pressed Esc while the
// turn ended (the turn's context cancelled).
func TestSessionCeremonyEndTurnSavesAfterCancellation(t *testing.T) {
	c, store, _, _ := sessionCeremonyFor(t, &fakeEngine{})
	s := turnSession(t)
	if err := s.BeginTurn("primero", turnTools()); err != nil {
		t.Fatal(err)
	}
	c.BeginTurn(context.Background(), &s)
	if err := s.CompleteAssistant(assistant("ok")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.EndTurn(ctx, &s); err != nil || len(store.states) != 1 {
		t.Fatalf("%v saves %d", err, len(store.states))
	}
}

// A turn whose record is larger than LastRecord's bound still completes
// and the instance takes the next turn (14 October 2026: the cut record
// failed Validate, and every later turn with it).
func TestSessionCeremonyBoundsAnOversizedRecord(t *testing.T) {
	engine := &fakeEngine{}
	c, _, _, _ := sessionCeremonyFor(t, engine)
	s := turnSession(t)
	if err := s.BeginTurn("muchos comandos", turnTools()); err != nil {
		t.Fatal(err)
	}
	c.BeginTurn(context.Background(), &s)
	var calls []root.ToolCall
	for i := 0; i < 12; i++ {
		calls = append(calls, root.ToolCall{ID: root.ToolCallID(fmt.Sprintf("c%d", i)), Name: "local_exec", Arguments: mustObject(t, `{"program":"go","args":["test"],"why":"check"}`)})
	}
	if err := s.CompleteAssistant(assistant("", calls...)); err != nil {
		t.Fatal(err)
	}
	output := strings.Repeat(`"quoted" `, 600)
	for _, call := range calls {
		content := fmt.Sprintf(`{"status":"completed","output":{"exit_code":0,"stdout":%q,"stderr":""}}`, output)
		if err := s.RecordToolOutcome(call.ID, domain.DecisionAutoApprove, domain.ToolOutcome{Content: root.Text(content)}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.CompleteAssistant(assistant("hecho")); err != nil {
		t.Fatal(err)
	}
	if err := c.EndTurn(context.Background(), &s); err != nil {
		t.Fatal(err)
	}
	instance, _ := s.Instance()
	if instance.Turns != 1 || instance.Fence != "" || instance.LastRecord == "" || len(instance.LastRecord) > domain.MaxSessionRecordBytes {
		t.Fatalf("turns %d fence %q record %d bytes", instance.Turns, instance.Fence, len(instance.LastRecord))
	}
	if err := s.BeginTurn("siguiente", turnTools()); err != nil {
		t.Fatal(err)
	}
	c.BeginTurn(context.Background(), &s)
	if instance, _ = s.Instance(); instance.Fence != "fence-turn" || instance.TurnIndex != len(s.Messages())-1 {
		t.Fatalf("next turn not claimed: %+v", instance)
	}
}

// An instance that started continues once MADE is back: starting its id
// again is refused, which left the session without a record for good. An
// open instance is marked unavailable when the successor its exhausted
// turns asked for could not be opened.
func TestSessionCeremonyContinuesItsInstanceAfterMADEWasUnavailable(t *testing.T) {
	engine := &fakeEngine{}
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
	opened, _ := s.Instance()
	opened.Unavailable = "connection refused"
	if err := s.SetInstance(opened); err != nil {
		t.Fatal(err)
	}
	engine.calls = nil
	engine.existing = map[string]bool{opened.Instance: true}
	if err := s.BeginTurn("tercero", turnTools()); err != nil {
		t.Fatal(err)
	}
	c.BeginTurn(context.Background(), &s)
	instance, _ := s.Instance()
	if instance.Unavailable != "" || instance.Instance != opened.Instance || instance.Fence != "fence-turn" {
		t.Fatalf("instance %+v", instance)
	}
	if strings.Contains(strings.Join(engine.calls, ";"), "start") {
		t.Fatalf("started again: %v", engine.calls)
	}
}

// A first instance MADE has already (the sidecar that named it was lost)
// continues in a successor, with the taken one as its predecessor.
func TestSessionCeremonyOpensASuccessorWhenTheFirstInstanceExists(t *testing.T) {
	engine := &fakeEngine{}
	c, _, _, _ := sessionCeremonyFor(t, engine)
	s := turnSession(t)
	first := instanceID(s.ID(), 1)
	engine.existing = map[string]bool{first: true}
	if err := s.BeginTurn("primero", turnTools()); err != nil {
		t.Fatal(err)
	}
	c.BeginTurn(context.Background(), &s)
	instance, _ := s.Instance()
	if instance.Instance != first+"-2" || instance.Unavailable != "" || engine.inputs["predecessor"] != first {
		t.Fatalf("instance %+v inputs %v", instance, engine.inputs)
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

// A KMP entry the engine does not take waits in the sidecar and is written
// at the next turn end, oldest first.
func TestSessionCeremonyKeepsAnEntryKMPRefusedForTheNextTurn(t *testing.T) {
	engine := &fakeEngine{}
	c, _, journal, _ := sessionCeremonyFor(t, engine)
	journal.err = errors.New("kmp down")
	s := turnSession(t)
	if err := s.BeginTurn("añade hola.txt y comprueba", turnTools()); err != nil {
		t.Fatal(err)
	}
	c.BeginTurn(context.Background(), &s)
	recordTurnOn(t, &s)
	if err := c.EndTurn(context.Background(), &s); err != nil {
		t.Fatal(err)
	}
	instance, _ := s.Instance()
	if len(instance.Pending) != 1 || len(journal.entries) != 1 {
		t.Fatalf("pending %d entries %d", len(instance.Pending), len(journal.entries))
	}
	journal.err = nil
	if err := s.BeginTurn("otra vez", turnTools()); err != nil {
		t.Fatal(err)
	}
	c.BeginTurn(context.Background(), &s)
	if err := s.CompleteAssistant(assistant("ok")); err != nil {
		t.Fatal(err)
	}
	if err := c.EndTurn(context.Background(), &s); err != nil {
		t.Fatal(err)
	}
	instance, _ = s.Instance()
	if len(instance.Pending) != 0 || len(journal.entries) != 2 || journal.entries[1].Key != journal.entries[0].Key {
		t.Fatalf("pending %d entries %+v", len(instance.Pending), journal.entries)
	}
}

type listerStub struct {
	*memoryStore
	sessions map[domain.SessionID]domain.Session
	held     map[domain.SessionID]bool
	released []domain.SessionID
}

func (l *listerStub) List(context.Context) ([]domain.SessionSummary, error) {
	var out []domain.SessionSummary
	for id, s := range l.sessions {
		out = append(out, domain.SessionSummary{ID: id, Workspace: s.Export().Workspace})
	}
	return out, nil
}
func (l *listerStub) Load(_ context.Context, id domain.SessionID) (domain.Session, error) {
	if l.held[id] {
		return domain.Session{}, errors.New("session already has a writer")
	}
	return l.sessions[id], nil
}
func (l *listerStub) Release(id domain.SessionID) error {
	l.released = append(l.released, id)
	return nil
}

// The sweep ends the open instances of sessions no console holds, leaves a
// session another console has open, and skips the current one.
func TestSweepAbandonedEndsOnlyUnheldOpenInstances(t *testing.T) {
	engine := &fakeEngine{}
	c, store, _, _ := sessionCeremonyFor(t, engine)
	lister := &listerStub{memoryStore: store, sessions: map[domain.SessionID]domain.Session{}, held: map[domain.SessionID]bool{}}
	workspace, elsewhere := domain.Workspace(t.TempDir()), domain.Workspace(t.TempDir())
	open := func(id string, fence string) domain.Session {
		s, err := domain.NewSession(domain.SessionID(id), workspace, "test/model")
		if err != nil {
			t.Fatal(err)
		}
		if err := s.BeginTurn("x", turnTools()); err != nil {
			t.Fatal(err)
		}
		if err := s.SetInstance(domain.SessionInstance{Instance: "axlr-session-" + id, Turns: 2, Fence: fence, TurnIndex: 0}); err != nil {
			t.Fatal(err)
		}
		return s
	}
	lister.sessions["a123456789abcdef0123456789abcdef"] = open("a123456789abcdef0123456789abcdef", "fence-turn")
	lister.sessions["b123456789abcdef0123456789abcdef"] = open("b123456789abcdef0123456789abcdef", "")
	lister.held["b123456789abcdef0123456789abcdef"] = true
	lister.sessions["c123456789abcdef0123456789abcdef"] = open("c123456789abcdef0123456789abcdef", "")
	other, _ := domain.NewSession("d123456789abcdef0123456789abcdef", elsewhere, "test/model")
	lister.sessions["d123456789abcdef0123456789abcdef"] = other
	ended, err := c.SweepAbandoned(context.Background(), lister, workspace, "c123456789abcdef0123456789abcdef")
	if err != nil || ended != 1 {
		t.Fatalf("ended %d %v calls %v", ended, err, engine.calls)
	}
	// a: the interrupted turn completed, then the abandoning turn and the transition.
	if strings.Join(engine.calls, ";") != "complete turn;claim turn 1791907200;complete turn;transition abandoned" {
		t.Fatalf("calls %v", engine.calls)
	}
	if engine.completed[0]["interrupted"] != true || engine.completed[1]["abandoned"] != true {
		t.Fatalf("completed %+v", engine.completed)
	}
	if len(lister.released) != 1 || lister.released[0] != "a123456789abcdef0123456789abcdef" {
		t.Fatalf("released %v", lister.released)
	}
}

type guideStub struct {
	calls   []string
	fail    map[string]error
	context int
}

func (g *guideStub) Guide(_ context.Context, registrationKey string, agentID domain.MemoryAgentID, contextKey string) (GuideResult, error) {
	call := "register " + registrationKey
	if registrationKey == "" {
		call = "context " + string(agentID) + " " + contextKey
	}
	g.calls = append(g.calls, call)
	if err := g.fail[call]; err != nil {
		return GuideResult{}, err
	}
	if registrationKey != "" {
		return GuideResult{AgentID: domain.MemoryAgentID("agent_" + registrationKey), ContextID: "ctx_registration"}, nil
	}
	g.context++
	return GuideResult{AgentID: agentID, ContextID: domain.MemoryContextID("ctx_" + itoa(g.context))}, nil
}

type agentsStub struct{ agents map[string]string }

func (a *agentsStub) Agent(_ context.Context, store string) (domain.MemoryAgentID, bool) {
	id, ok := a.agents[store]
	return domain.MemoryAgentID(id), ok
}

func (a *agentsStub) Remember(_ context.Context, store string, agentID domain.MemoryAgentID, _ string) error {
	a.agents[store] = string(agentID)
	return nil
}

func identityFor(guide *guideStub, agents *agentsStub) *MemoryIdentity {
	return &MemoryIdentity{Store: "/repo/.kernel", RegistrationKey: "axlr:install-1", Agents: agents, Guide: guide}
}

// The first prompt in a store registers the console's agent once and opens
// one context per session; the next session reuses the agent.
func TestSessionCeremonyRegistersTheAgentOnceAndOpensAContextPerSession(t *testing.T) {
	engine := &fakeEngine{}
	c, _, _, _ := sessionCeremonyFor(t, engine)
	guide, agents := &guideStub{}, &agentsStub{agents: map[string]string{}}
	c.Identity = identityFor(guide, agents)
	s := turnSession(t)
	if err := s.BeginTurn("hola", turnTools()); err != nil {
		t.Fatal(err)
	}
	c.BeginTurn(context.Background(), &s)
	instance, _ := s.Instance()
	if instance.MemoryAgent != "agent_axlr:install-1" || instance.MemoryContext != "ctx_1" || instance.Instance == "" {
		t.Fatalf("instance %+v", instance)
	}
	if strings.Join(guide.calls, ";") != "register axlr:install-1;context agent_axlr:install-1 axlr:session:"+string(s.ID()) || agents.agents["/repo/.kernel"] != "agent_axlr:install-1" {
		t.Fatalf("calls %v agents %v", guide.calls, agents.agents)
	}
	// The second prompt opens nothing more.
	if err := s.CompleteAssistant(assistant("hola")); err != nil {
		t.Fatal(err)
	}
	_ = c.EndTurn(context.Background(), &s)
	if err := s.BeginTurn("otra", turnTools()); err != nil {
		t.Fatal(err)
	}
	c.BeginTurn(context.Background(), &s)
	if instance, _ = s.Instance(); instance.MemoryContext != "ctx_1" || len(guide.calls) != 2 {
		t.Fatalf("instance %+v calls %v", instance, guide.calls)
	}
	// Another session in the same store reuses the agent.
	other, err := domain.NewSession("fedcba9876543210fedcba9876543210", s.Export().Workspace, "test/model")
	if err != nil {
		t.Fatal(err)
	}
	if err := other.BeginTurn("hola", turnTools()); err != nil {
		t.Fatal(err)
	}
	c.BeginTurn(context.Background(), &other)
	if instance, _ = other.Instance(); instance.MemoryContext != "ctx_2" || guide.calls[2] != "context agent_axlr:install-1 axlr:session:fedcba9876543210fedcba9876543210" {
		t.Fatalf("instance %+v calls %v", instance, guide.calls)
	}
}

// An agent KMP no longer knows (the store emptied) is registered again; a
// registration that fails leaves the session without a context and the
// turn runs.
func TestSessionCeremonyRegistersAgainWhenTheStoreForgotTheAgent(t *testing.T) {
	engine := &fakeEngine{}
	c, _, _, _ := sessionCeremonyFor(t, engine)
	guide := &guideStub{fail: map[string]error{"context stale axlr:session:0123456789abcdef0123456789abcdef": errors.New("not_found: agent stale")}}
	agents := &agentsStub{agents: map[string]string{"/repo/.kernel": "stale"}}
	c.Identity = identityFor(guide, agents)
	s := turnSession(t)
	if err := s.BeginTurn("hola", turnTools()); err != nil {
		t.Fatal(err)
	}
	c.BeginTurn(context.Background(), &s)
	instance, _ := s.Instance()
	if instance.MemoryAgent != "agent_axlr:install-1" || instance.MemoryContext != "ctx_1" || agents.agents["/repo/.kernel"] != "agent_axlr:install-1" || len(guide.calls) != 3 {
		t.Fatalf("instance %+v calls %v agents %v", instance, guide.calls, agents.agents)
	}
	var logged []string
	c.Log = func(line string) { logged = append(logged, line) }
	guide = &guideStub{fail: map[string]error{"register axlr:install-1": errors.New("engine down")}}
	c.Identity = identityFor(guide, &agentsStub{agents: map[string]string{}})
	other, err := domain.NewSession("fedcba9876543210fedcba9876543210", s.Export().Workspace, "test/model")
	if err != nil {
		t.Fatal(err)
	}
	if err := other.BeginTurn("hola", turnTools()); err != nil {
		t.Fatal(err)
	}
	c.BeginTurn(context.Background(), &other)
	if instance, _ = other.Instance(); instance.MemoryContext != "" || instance.Instance == "" || instance.Fence == "" {
		t.Fatalf("instance %+v", instance)
	}
	if len(logged) == 0 || !strings.Contains(logged[0], "engine down") {
		t.Fatalf("logged %v", logged)
	}
}

// Without MADE the session still gets its KMP context, and the turn's
// entry carries it.
func TestSessionCeremonyOpensTheContextWithoutMADE(t *testing.T) {
	c, store, _, _ := sessionCeremonyFor(t, nil)
	c.Engine = nil
	guide, agents := &guideStub{}, &agentsStub{agents: map[string]string{}}
	c.Identity = identityFor(guide, agents)
	s := turnSession(t)
	if err := s.BeginTurn("hola", turnTools()); err != nil {
		t.Fatal(err)
	}
	c.BeginTurn(context.Background(), &s)
	instance, has := s.Instance()
	if !has || instance.MemoryContext != "ctx_1" || instance.Instance != "" || instance.Unavailable != "" {
		t.Fatalf("instance %+v", instance)
	}
	if err := c.EndTurn(context.Background(), &s); err != nil || len(store.states) != 0 {
		t.Fatalf("no MADE record without MADE: %v %d", err, len(store.states))
	}
	if _, err := domain.RestoreSession(s.Export()); err != nil {
		t.Fatal(err)
	}
}

func TestSessionCeremonyTurnEntryCarriesTheContext(t *testing.T) {
	engine := &fakeEngine{}
	c, _, journal, _ := sessionCeremonyFor(t, engine)
	c.Identity = identityFor(&guideStub{}, &agentsStub{agents: map[string]string{}})
	s := turnSession(t)
	if err := s.BeginTurn("añade hola.txt y comprueba", turnTools()); err != nil {
		t.Fatal(err)
	}
	c.BeginTurn(context.Background(), &s)
	recordTurnOn(t, &s)
	if err := c.EndTurn(context.Background(), &s); err != nil {
		t.Fatal(err)
	}
	if len(journal.entries) != 1 || journal.entries[0].ContextID != "ctx_1" {
		t.Fatalf("journal %+v", journal.entries)
	}
}

// A closed instance continues in its successor when the session is
// resumed (or imported): MADE refuses to start the same id again.
func TestSessionCeremonyContinuesAClosedInstanceInItsSuccessor(t *testing.T) {
	engine := &fakeEngine{}
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
	if err := c.Close(context.Background(), &s, "exit"); err != nil {
		t.Fatal(err)
	}
	closed, _ := s.Instance()
	closed.Pending = []string{encodeEntry(TurnEntry{About: "ws:x", Key: "k", Summary: "left over"})}
	if err := s.SetInstance(closed); err != nil {
		t.Fatal(err)
	}
	engine.existing = map[string]bool{closed.Instance + "-2": true}
	engine.calls = nil
	if err := s.BeginTurn("segundo", turnTools()); err != nil {
		t.Fatal(err)
	}
	c.BeginTurn(context.Background(), &s)
	instance, _ := s.Instance()
	if instance.Closed || instance.Instance != closed.Instance+"-3" || instance.Turns != 0 || len(instance.Pending) != 1 {
		t.Fatalf("successor %+v after %+v", instance, closed)
	}
	if !strings.HasPrefix(strings.Join(engine.calls, ";"), "start refused "+closed.Instance+"-2;start axlr_session 1.0 about=;claim turn 2") || engine.inputs["predecessor"] != closed.Instance {
		t.Fatalf("calls %v inputs %v", engine.calls, engine.inputs)
	}
}
