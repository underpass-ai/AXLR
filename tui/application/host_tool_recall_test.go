package application

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/domain"
)

func recallIdentity(t *testing.T) domain.ToolIdentity {
	t.Helper()
	id, err := domain.NewHostToolIdentity(domain.HostOperationRecall)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestHostRecallBuildsTheRequestFromTheSession(t *testing.T) {
	s := rememberSession(t, domain.ModeNormal)
	if err := s.SetInstance(domain.SessionInstance{KMPContext: "ctx_1"}); err != nil {
		t.Fatal(err)
	}
	labels := &contextLabels{labels: map[domain.SessionID]domain.SessionLabel{s.Export().ID: {About: "project:AXLR"}}}
	recall := &recallStub{texts: map[string]string{"project:AXLR": "- the parser keeps dates in UTC"}}
	u := HostToolUseCase{Recall: recall, Labels: labels}
	run := func(arguments string) map[string]any {
		t.Helper()
		outcome, err := u.Execute(context.Background(), s, recallIdentity(t), mustObject(t, arguments))
		if err != nil {
			t.Fatal(err)
		}
		var answer map[string]any
		if err := json.Unmarshal([]byte(outcome.Content), &answer); err != nil {
			t.Fatalf("%v: %s", err, outcome.Content)
		}
		if outcome.IsError {
			answer["_error"] = true
		}
		return answer
	}
	answer := run(`{"why":"see where the parser stands"}`)
	if answer["mode"] != RecallWake || answer["about"] != "project:AXLR" || answer["recorded"] != true || answer["memory"] != "- the parser keeps dates in UTC" || recall.requests[0].ContextID != "ctx_1" {
		t.Fatalf("wake %+v %+v", answer, recall.requests[0])
	}
	run(`{"question":"what timezone does the parser use?"}`)
	run(`{"question":"what changed?","mode":"history","since":"2026-10-01","about":"project:other"}`)
	ask, history := recall.requests[1], recall.requests[2]
	if ask.Mode != RecallAsk || ask.Intent != "what timezone does the parser use?" || ask.About != "project:AXLR" || history.Mode != RecallHistory || history.Since != "2026-10-01T00:00:00Z" || history.About != "project:other" || history.Entries != recallHistoryEntries {
		t.Fatalf("ask %+v history %+v", ask, history)
	}
	empty := run(`{"about":"user:nobody"}`)
	if empty["recorded"] != false || empty["memory"] != "nothing recorded under user:nobody" {
		t.Fatalf("empty %+v", empty)
	}
	for _, refused := range []string{`{"mode":"ask"}`, `{"mode":"trace"}`, `{"since":"yesterday"}`, `{"q":"x"}`} {
		if answer := run(refused); answer["_error"] != true {
			t.Fatalf("accepted %s: %+v", refused, answer)
		}
	}
	if outcome, _ := (HostToolUseCase{}).Execute(context.Background(), s, recallIdentity(t), mustObject(t, `{}`)); !outcome.IsError || !strings.Contains(string(outcome.Content), "KMP is not connected") {
		t.Fatalf("%+v", outcome)
	}
}

// The ask and history answers pass through compact.
func TestHostRecallPassesCompactAnswers(t *testing.T) {
	s := rememberSession(t, domain.ModeNormal)
	answers := recallAnswers{answer: map[string]any{"answer": "UNKNOWN", "answer_status": "unknown"}}
	u := HostToolUseCase{Recall: answers}
	outcome, err := u.Execute(context.Background(), s, recallIdentity(t), mustObject(t, `{"question":"anything about elephants?"}`))
	if err != nil || outcome.IsError {
		t.Fatalf("%+v %v", outcome, err)
	}
	var answer map[string]any
	if err := json.Unmarshal([]byte(outcome.Content), &answer); err != nil {
		t.Fatal(err)
	}
	if answer["answer"] != "UNKNOWN" || answer["answer_status"] != "unknown" || answer["recorded"] != true || answer["mode"] != RecallAsk {
		t.Fatalf("%+v", answer)
	}
}

type recallAnswers struct{ answer map[string]any }

func (r recallAnswers) Recall(_ context.Context, request RecallRequest) (RecallResult, error) {
	return RecallResult{About: request.About, Mode: request.Mode, Answer: r.answer}, nil
}

// axlr_recall is offered whenever KMP is connected, whatever the mode, and
// approved without a card: it reads.
func TestRecallToolIsOfferedWithKMPAndReadsFreely(t *testing.T) {
	tool := RecallTool()
	if tool.Definition.Name != HostRecallName || tool.Identity.LocalOperation != domain.HostOperationRecall {
		t.Fatalf("%+v", tool)
	}
	if !automaticallyApproves(nil, tool.Identity) {
		t.Fatal("a read needs no card")
	}
	for _, mode := range []domain.WorkMode{domain.ModeNormal, domain.ModePlan, domain.ModeTask, domain.ModeRepair} {
		if verdict, reason := mode.Judge(tool.Identity, mustObject(t, `{}`)); verdict == domain.VerdictDeny {
			t.Fatalf("%s refuses a read: %s", mode, reason)
		}
	}
	if sideEffectTools[HostRecallName] {
		t.Fatal("a recall is a read in the turn record")
	}
}
