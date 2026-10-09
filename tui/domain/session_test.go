package domain

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	axlr "github.com/underpass-ai/AXLR/domain"
)

func session(t *testing.T) Session {
	t.Helper()
	s, err := NewSession("0123456789abcdef0123456789abcdef", Workspace(t.TempDir()), "test/model")
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func object(t *testing.T) axlr.JSONValue {
	t.Helper()
	v, e := axlr.NewJSONObject([]byte(`{}`))
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func catalog(t *testing.T) []AvailableTool {
	t.Helper()
	id, e := NewLocalToolIdentity("read")
	if e != nil {
		t.Fatal(e)
	}
	return []AvailableTool{{Definition: axlr.ToolDefinition{Name: "read", Parameters: object(t)}, Identity: id}}
}
func completion(t *testing.T, ids ...string) axlr.CompletionResult {
	t.Helper()
	m := axlr.Message{Role: axlr.RoleAssistant, Content: "answer"}
	for _, id := range ids {
		m.ToolCalls = append(m.ToolCalls, axlr.ToolCall{ID: axlr.ToolCallID(id), Name: "read", Arguments: object(t)})
	}
	return axlr.CompletionResult{Message: m, FinishReason: "stop"}
}
func must(t *testing.T, e error) {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
}
func TestIdentityValidation(t *testing.T) {
	for _, raw := range []string{"", strings.Repeat("a", 31), strings.Repeat("a", 33), strings.Repeat("A", 32), strings.Repeat("z", 32)} {
		if _, e := NewSessionID(raw); e == nil {
			t.Errorf("accepted ID %q", raw)
		}
	}
	if _, e := NewSessionID(strings.Repeat("a", 32)); e != nil {
		t.Fatal(e)
	}
	for _, raw := range []string{"", "relative", "/bad\x00path"} {
		if _, e := NewWorkspace(raw); e == nil {
			t.Errorf("accepted workspace %q", raw)
		}
	}
	if _, e := NewWorkspace(t.TempDir()); e != nil {
		t.Fatal(e)
	}
	for _, op := range []string{"read", "write", "edit", "exec"} {
		if _, e := NewLocalToolIdentity(op); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := NewLocalToolIdentity("delete"); e == nil {
		t.Fatal("unknown operation accepted")
	}
	if _, e := NewPluginToolIdentity(axlr.PluginRef{PluginID: "files", ToolName: "read"}); e != nil {
		t.Fatal(e)
	}
	if _, e := NewPluginToolIdentity(axlr.PluginRef{}); e == nil {
		t.Fatal("empty plugin accepted")
	}
	s := session(t)
	state := s.Export()
	for _, change := range []func(*SessionState){func(s *SessionState) { s.ID = "bad" }, func(s *SessionState) { s.Workspace = "relative" }, func(s *SessionState) { s.Model = "" }} {
		bad := state
		change(&bad)
		if _, e := NewSession(bad.ID, bad.Workspace, bad.Model); e == nil {
			t.Fatal("invalid construction accepted")
		}
	}
}

func TestChangeModelAllowedStatesPreserveHistory(t *testing.T) {
	for _, status := range []SessionStatus{StatusIdle, StatusComplete, StatusInterrupted} {
		t.Run(string(status), func(t *testing.T) {
			s := session(t)
			if status != StatusIdle {
				must(t, s.BeginTurn("hello", nil))
				if status == StatusComplete {
					must(t, s.CompleteAssistant(completion(t)))
				} else {
					must(t, s.InterruptDraft("partial"))
				}
			}
			before := s.Export()
			must(t, s.ChangeModel("next/model"))
			got := s.Export()
			if got.Model != "next/model" {
				t.Fatalf("model: %q", got.Model)
			}
			got.Model = before.Model
			if !reflect.DeepEqual(got, before) {
				t.Fatalf("model change modified history or status: %+v", s.Export())
			}
		})
	}
}

func TestChangeModelRejectsInvalidBusyAndPending(t *testing.T) {
	for _, state := range []string{"invalid", "streaming", "approval", "interrupted-pending"} {
		t.Run(state, func(t *testing.T) {
			s := session(t)
			if state != "invalid" {
				must(t, s.BeginTurn("hello", catalog(t)))
			}
			if state == "approval" || state == "interrupted-pending" {
				must(t, s.CompleteAssistant(completion(t, "one")))
			}
			if state == "interrupted-pending" {
				must(t, s.PauseTurn())
			}
			before := s.Export()
			model := axlr.ModelID("next/model")
			if state == "invalid" {
				model = ""
			}
			if err := s.ChangeModel(model); err == nil {
				t.Fatal("accepted forbidden model change")
			}
			if !reflect.DeepEqual(s.Export(), before) {
				t.Fatal("rejected change mutated session")
			}
		})
	}
}
func TestOrderedTurnAndDecisions(t *testing.T) {
	s := session(t)
	if s.Status() != StatusIdle {
		t.Fatal(s.Status())
	}
	must(t, s.BeginTurn("hello", catalog(t)))
	must(t, s.CompleteAssistant(completion(t, "one", "two")))
	if s.Status() != StatusApproval || len(s.Pending()) != 2 {
		t.Fatal("calls not queued")
	}
	if e := s.RecordToolOutcome("two", DecisionApprove, ToolOutcome{Content: "second"}); e == nil {
		t.Fatal("out of order execution accepted")
	}
	must(t, s.RecordToolOutcome("one", DecisionDeny, ToolOutcome{Content: "denied", IsError: true}))
	must(t, s.RecordToolOutcome("two", DecisionApprove, ToolOutcome{Content: "second", Uncertain: true}))
	if s.Status() != StatusStreaming {
		t.Fatal(s.Status())
	}
	must(t, s.CompleteAssistant(completion(t)))
	m := s.Messages()
	if len(m) != 5 || m[1].Role != axlr.RoleAssistant || len(m[1].ToolCalls) != 2 || m[2].ToolCallID != "one" || m[3].ToolCallID != "two" || s.Status() != StatusComplete {
		t.Fatalf("bad transcript: %+v", m)
	}
	st := s.Export()
	if !st.Activity[1].Outcome.Uncertain || st.Activity[0].Decision != DecisionDeny {
		t.Fatal("outcome metadata lost")
	}
	must(t, s.BeginTurn("again", catalog(t)))
}
func TestIllegalTransitionsAreAtomic(t *testing.T) {
	s := session(t)
	reject := func(f func() error) {
		t.Helper()
		before := s.Export()
		if f() == nil {
			t.Fatal("accepted illegal transition")
		}
		if !reflect.DeepEqual(before, s.Export()) {
			t.Fatal("failed operation mutated state")
		}
	}
	reject(func() error { return s.CompleteAssistant(completion(t)) })
	reject(func() error { return s.CancelPending() })
	reject(func() error { return s.InterruptDraft("partial") })
	reject(func() error { return s.BeginTurn("", catalog(t)) })
	bad := catalog(t)
	bad[0].Identity = ToolIdentity{}
	reject(func() error { return s.BeginTurn("hi", bad) })
	must(t, s.BeginTurn("hi", catalog(t)))
	reject(func() error { return s.BeginTurn("again", catalog(t)) })
	wrong := completion(t)
	wrong.Message.Role = axlr.RoleUser
	reject(func() error { return s.CompleteAssistant(wrong) })
	must(t, s.CompleteAssistant(completion(t, "one")))
	reject(func() error { return s.CompleteAssistant(completion(t)) })
	reject(func() error { return s.InterruptDraft("partial") })
	reject(func() error { return s.RecordToolOutcome("one", "invalid", ToolOutcome{}) })
	reject(func() error { return s.RecordToolOutcome("unknown", DecisionApprove, ToolOutcome{}) })
}
func TestDuplicateIDsAndTurnLimit(t *testing.T) {
	s := session(t)
	must(t, s.BeginTurn("hi", catalog(t)))
	before := s.Export()
	if e := s.CompleteAssistant(completion(t, "same", "same")); e == nil {
		t.Fatal("duplicates accepted")
	}
	if !reflect.DeepEqual(before, s.Export()) {
		t.Fatal("duplicate mutated state")
	}
	for i := 0; i < 32; i++ {
		id := fmt.Sprint(i)
		must(t, s.CompleteAssistant(completion(t, id)))
		must(t, s.RecordToolOutcome(axlr.ToolCallID(id), DecisionApprove, ToolOutcome{Content: "ok"}))
	}
	if e := s.CompleteAssistant(completion(t, "33")); e == nil {
		t.Fatal("33rd call accepted")
	}
	if s.Status() != StatusInterrupted || len(s.Pending()) != 0 {
		t.Fatal("limit did not pause turn")
	}
	must(t, s.BeginTurn("next", catalog(t)))
	if e := s.CompleteAssistant(completion(t, "0")); e == nil {
		t.Fatal("historical duplicate accepted")
	}
}
func TestCancellationAndInterruptedDraft(t *testing.T) {
	s := session(t)
	must(t, s.BeginTurn("hi", catalog(t)))
	must(t, s.CompleteAssistant(completion(t, "one", "two", "three")))
	must(t, s.RecordToolOutcome("one", DecisionApprove, ToolOutcome{Content: "done"}))
	must(t, s.CancelPending())
	if s.Status() != StatusInterrupted || len(s.Pending()) != 0 {
		t.Fatal("cancel left pending work")
	}
	m := s.Messages()
	if len(m) != 5 || m[2].Content != "done" || m[3].ToolCallID != "two" || m[4].ToolCallID != "three" || !strings.Contains(string(m[3].Content), "cancel") {
		t.Fatal(m)
	}
	must(t, s.BeginTurn("again", catalog(t)))
	before := s.Messages()
	must(t, s.InterruptDraft("partial"))
	if !reflect.DeepEqual(before, s.Messages()) || s.Export().Draft != "partial" {
		t.Fatal("draft entered model history")
	}
}

func TestStartingNewTurnArchivesInterruptedAnswerOutsideModelHistory(t *testing.T) {
	s := session(t)
	must(t, s.BeginTurn("first question", nil))
	must(t, s.InterruptDraft("first partial answer"))
	must(t, s.BeginTurn("second question", nil))
	state := s.Export()
	if len(state.Messages) != 2 || state.Messages[0].Role != axlr.RoleUser || state.Messages[1].Role != axlr.RoleUser {
		t.Fatalf("model history changed: %+v", state.Messages)
	}
	if state.Draft != "" || len(state.ArchivedDrafts) != 1 || state.ArchivedDrafts[0].AfterMessage != 1 || state.ArchivedDrafts[0].Content != "first partial answer" {
		t.Fatalf("interrupted answer was not archived at its turn: %+v", state)
	}
	must(t, s.InterruptDraft("second partial answer"))
	must(t, s.BeginTurn("third question", nil))
	state = s.Export()
	if len(state.ArchivedDrafts) != 2 || state.ArchivedDrafts[1].AfterMessage != 2 {
		t.Fatalf("archived drafts lost order: %+v", state.ArchivedDrafts)
	}
	restored, err := RestoreSession(state)
	must(t, err)
	state.Status = StatusInterrupted
	if !reflect.DeepEqual(restored.Export(), state) {
		t.Fatal("restoring the session lost archived answers")
	}
}
func TestRestorePendingAndCopySafety(t *testing.T) {
	s := session(t)
	tools := catalog(t)
	must(t, s.BeginTurn("hi", tools))
	tools[0].Definition.Name = "changed"
	c := completion(t, "one", "two")
	must(t, s.CompleteAssistant(c))
	c.Message.ToolCalls[0].ID = "changed"
	state := s.Export()
	restored, e := RestoreSession(state)
	must(t, e)
	if restored.Status() != StatusInterrupted || len(restored.Pending()) != 2 || len(restored.Messages()) != 2 {
		t.Fatal("restore executed or lost calls")
	}
	if e := restored.RecordToolOutcome("one", DecisionApprove, ToolOutcome{Content: "ok"}); e == nil {
		t.Fatal("loaded call executed without resume")
	}
	if e := restored.BeginTurn("new", catalog(t)); e == nil {
		t.Fatal("unresolved calls bypassed")
	}
	must(t, restored.CancelPending())
	must(t, restored.BeginTurn("new", catalog(t)))
	state.Messages[1].ToolCalls[0].ID = "bad"
	state.ToolSnapshot[0].Definition.Name = "bad"
	state.Activity[0].Call.ID = "bad"
	messages := s.Messages()
	messages[1].ToolCalls[0].ID = "bad"
	pending := s.Pending()
	pending[0].Call.ID = "bad"
	snapshot := s.ToolSnapshot()
	snapshot[0].Definition.Name = "bad"
	if s.Messages()[1].ToolCalls[0].ID != "one" || s.Pending()[0].Call.ID != "one" || s.ToolSnapshot()[0].Definition.Name != "read" {
		t.Fatal("mutable state escaped")
	}
}
func TestRestoreRejectsMalformedState(t *testing.T) {
	s := session(t)
	must(t, s.BeginTurn("hi", catalog(t)))
	must(t, s.CompleteAssistant(completion(t, "one")))
	for name, change := range map[string]func(*SessionState){"id": func(s *SessionState) { s.ID = "bad" }, "status": func(s *SessionState) { s.Status = "bad" }, "count": func(s *SessionState) { s.TurnCallCount = 0 }, "missing activity": func(s *SessionState) { s.Activity = nil }, "false completion": func(s *SessionState) { s.Status = StatusComplete }, "orphan result": func(s *SessionState) {
		s.Messages = append(s.Messages, axlr.Message{Role: axlr.RoleTool, ToolCallID: "other"})
	}, "invalid draft": func(s *SessionState) { s.Draft = "\x00" }} {
		t.Run(name, func(t *testing.T) {
			state := s.Export()
			change(&state)
			if _, e := RestoreSession(state); e == nil {
				t.Fatal("malformed state accepted")
			}
		})
	}
}

func TestRestoreEveryStableTransition(t *testing.T) {
	s := session(t)
	roundTrip := func(want SessionStatus) {
		t.Helper()
		state := s.Export()
		restored, e := RestoreSession(state)
		must(t, e)
		if restored.Status() != want {
			t.Fatalf("restored status %q, want %q", restored.Status(), want)
		}
		got := restored.Export()
		state.Status = want
		if !reflect.DeepEqual(got, state) {
			t.Fatalf("round trip changed state: %+v", got)
		}
	}
	roundTrip(StatusIdle)
	must(t, s.BeginTurn("first", catalog(t)))
	roundTrip(StatusInterrupted)
	must(t, s.CompleteAssistant(completion(t, "one", "two")))
	roundTrip(StatusInterrupted)
	must(t, s.RecordToolOutcome("one", DecisionApprove, ToolOutcome{Content: "done", Uncertain: true}))
	roundTrip(StatusInterrupted)
	must(t, s.CancelPending())
	roundTrip(StatusInterrupted)
	must(t, s.BeginTurn("second", nil))
	must(t, s.InterruptDraft("partial"))
	roundTrip(StatusInterrupted)
	must(t, s.BeginTurn("third", catalog(t)))
	must(t, s.CompleteAssistant(completion(t)))
	roundTrip(StatusComplete)
}

func TestOutcomeAndSessionValueCopiesDoNotAlias(t *testing.T) {
	s := session(t)
	must(t, s.BeginTurn("hi", catalog(t)))
	must(t, s.CompleteAssistant(completion(t, "one", "two")))
	copied := s
	must(t, s.RecordToolOutcome("one", DecisionApprove, ToolOutcome{Content: "done"}))
	if len(copied.Pending()) != 2 || len(copied.Messages()) != 2 {
		t.Fatal("session value copy mutated")
	}
	state := s.Export()
	restored, e := RestoreSession(state)
	must(t, e)
	state.Activity[0].Outcome.Content = "mutated"
	if restored.Export().Activity[0].Outcome.Content != "done" || s.Export().Activity[0].Outcome.Content != "done" {
		t.Fatal("outcome pointer aliases")
	}
}

func TestInvalidCompletionAndCatalogDoNotChangeHistory(t *testing.T) {
	s := session(t)
	tools := catalog(t)
	tools = append(tools, tools[0])
	if e := s.BeginTurn("hi", tools); e == nil {
		t.Fatal("duplicate catalog names accepted")
	}
	tools = catalog(t)
	tools[0].Identity.Plugin = axlr.PluginRef{PluginID: "plugin", ToolName: "read"}
	if e := s.BeginTurn("hi", tools); e == nil {
		t.Fatal("mixed identity accepted")
	}
	must(t, s.BeginTurn("hi", catalog(t)))
	before := s.Export()
	bad := completion(t, "one")
	bad.Message.ToolCalls[0].Arguments = axlr.JSONValue{}
	if e := s.CompleteAssistant(bad); e == nil {
		t.Fatal("invalid arguments accepted")
	}
	if !reflect.DeepEqual(before, s.Export()) {
		t.Fatal("invalid completion changed history")
	}
	ids := make([]string, 33)
	for i := range ids {
		ids[i] = fmt.Sprint(i)
	}
	if e := s.CompleteAssistant(completion(t, ids...)); e != ErrToolCallLimit {
		t.Fatal(e)
	}
	// The oversized answer is kept; none of its calls can run.
	if len(s.Messages()) != 2+len(ids) || len(s.Pending()) != 0 || s.Status() != StatusInterrupted {
		t.Fatalf("oversized batch: %d messages, %d pending, %s", len(s.Messages()), len(s.Pending()), s.Status())
	}
	for _, record := range s.Export().Activity {
		if record.Decision != DecisionDeny || record.Outcome == nil || !record.Outcome.IsError {
			t.Fatalf("a call over the limit was left to run: %+v", record)
		}
	}
}

// At the limit the answer stays in the transcript with its calls answered as
// not run; resuming restarts the budget from the calls already made, and
// both states restore from the transcript they keep.
func TestAnAnswerOverTheCallLimitIsKeptAndResumeRestartsTheBudget(t *testing.T) {
	for _, ceremony := range []bool{false, true} {
		s := session(t)
		must(t, s.BeginTurn("hi", catalog(t)))
		limit := MaxTurnToolCalls
		if ceremony {
			run := CeremonyRun{Definition: "axlr_debug", Version: "2.0", Instance: "i", Step: "repair", Iteration: 1, Compact: true}
			must(t, s.SetCeremony(run))
			limit = run.StepCallLimit()
		}
		batch := func(prefix string, n int) []string {
			ids := make([]string, n)
			for i := range ids {
				ids[i] = fmt.Sprintf("%s%d", prefix, i)
			}
			return ids
		}
		first := batch("a", limit-1)
		must(t, s.CompleteAssistant(completion(t, first...)))
		for _, id := range first {
			must(t, s.RecordToolOutcome(axlr.ToolCallID(id), DecisionApprove, ToolOutcome{Content: "ok"}))
		}
		if e := s.CompleteAssistant(completion(t, batch("b", 2)...)); e != ErrToolCallLimit {
			t.Fatalf("ceremony=%v: %v", ceremony, e)
		}
		if _, e := RestoreSession(s.Export()); e != nil {
			t.Fatalf("ceremony=%v: the kept answer does not restore: %v", ceremony, e)
		}
		last := s.Export().Activity[len(s.Export().Activity)-1].Outcome
		if !strings.Contains(string(last.Content), fmt.Sprintf("not run: the turn reached its %d tool-call limit", limit)) {
			t.Fatalf("ceremony=%v: %q", ceremony, last.Content)
		}
		must(t, s.ResumeTurn())
		if _, e := RestoreSession(s.Export()); e != nil {
			t.Fatalf("ceremony=%v: a resumed turn does not restore: %v", ceremony, e)
		}
		if e := s.CompleteAssistant(completion(t, batch("c", limit)...)); e != nil {
			t.Fatalf("ceremony=%v: the resumed turn has no budget: %v", ceremony, e)
		}
	}
}

func TestExplicitResumeRestoresApprovalWithoutExecuting(t *testing.T) {
	s := session(t)
	if e := s.ResumePending(); e == nil {
		t.Fatal("idle session resumed")
	}
	must(t, s.BeginTurn("hi", catalog(t)))
	if e := s.ResumePending(); e == nil {
		t.Fatal("stream without calls resumed")
	}
	must(t, s.CompleteAssistant(completion(t, "one", "two")))
	restored, e := RestoreSession(s.Export())
	must(t, e)
	before := restored.Messages()
	must(t, restored.ResumePending())
	if restored.Status() != StatusApproval || len(restored.Pending()) != 2 || !reflect.DeepEqual(before, restored.Messages()) {
		t.Fatal("resume executed or lost calls")
	}
	if e := restored.ResumePending(); e == nil {
		t.Fatal("approval resumed twice")
	}
	must(t, restored.RecordToolOutcome("one", DecisionDeny, ToolOutcome{Content: "denied"}))
	must(t, restored.RecordToolOutcome("two", DecisionApprove, ToolOutcome{Content: "done"}))
	if restored.Status() != StatusStreaming {
		t.Fatal(restored.Status())
	}
}

func TestRestoreAcceptsEmptyPersistedCollections(t *testing.T) {
	s := session(t)
	state := s.Export()
	state.Messages = []axlr.Message{}
	state.ToolSnapshot = []AvailableTool{}
	state.Activity = []PendingTool{}
	restored, e := RestoreSession(state)
	must(t, e)
	if restored.Status() != StatusIdle {
		t.Fatal(restored.Status())
	}
}
