package ceremonyhost

import (
	"context"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/tui/application"
)

// KMP 1.0 has no agent context: context_id is refused as an unknown
// argument on every tool, so no write the console makes carries one.
func TestWritesSendNoContext(t *testing.T) {
	var requests []map[string]any
	m := kmpAnswers(t, &requests,
		map[string]any{"accepted": true, "status": "committed", "local_refs": map[string]any{"turn": "ws:s1:entry:turn:x"}},
		map[string]any{"accepted": true, "status": "committed", "local_refs": map[string]any{"m1": "project:probe:entry:constraint:y"}},
		map[string]any{"accepted": true, "status": "committed", "local_refs": map[string]any{"settled": "project:probe:entry:decision:z"}})
	entry := application.TurnEntry{About: "ws:s1", Key: "axlr:turn:s1:0", Summary: "Turn 1: 1 file written (a.go).", Evidence: []string{"MADE axlr-session-s1 step turn #1"}, OccurredAt: time.Now()}
	if _, err := m.RecordTurn(context.Background(), entry); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Remember(context.Background(), rememberRequest()); err != nil {
		t.Fatal(err)
	}
	if _, err := m.RecordSettled(context.Background(), application.SettledEntry{About: "project:probe", Kind: "decision", Text: "t", SummaryEN: "t", Evidence: []string{"e"}, Key: "axlr:settled:s:0:1"}); err != nil {
		t.Fatal(err)
	}
	for _, request := range requests {
		if _, has := request["context_id"]; has {
			t.Fatalf("request %+v", request)
		}
	}
}

func TestRecordSettledWritesTheSelectedMemoryAndResumesItsReview(t *testing.T) {
	var requests []map[string]any
	m := kmpAnswers(t, &requests,
		map[string]any{"accepted": false, "status": "needs_review", "continuation": "review-1"},
		map[string]any{"accepted": true, "status": "committed", "local_refs": map[string]any{"settled": "project:probe:entry:decision:utc"}})
	entry := application.SettledEntry{About: "project:probe", Kind: "decision", Text: "Las fechas se guardan en UTC.", SummaryEN: "decision settled in turn 1: \"Las fechas se guardan en UTC.\"", Evidence: []string{"MADE i step turn #1", "local_write parser.go"}, Labels: map[string][]string{"judged_by": {"jev"}}, Key: "axlr:settled:s:0:1", Links: []application.MemoryLink{{Ref: "project:probe:entry:decision:old", Rel: "supersedes", Why: "Jev judged (p=0.60) that the turn's result replaces it", Evidence: "local_write parser.go"}}}
	ref, err := m.RecordSettled(context.Background(), entry)
	if err != nil || ref != "project:probe:entry:decision:utc" || len(requests) != 2 {
		t.Fatalf("%q %v %d", ref, err, len(requests))
	}
	memory := requests[0]["memories"].([]any)[0].(map[string]any)
	link := memory["connect_to"].([]any)[0].(map[string]any)
	if memory["kind"] != "decision" || memory["summary"] != entry.Text || memory["summary_en"] != entry.SummaryEN || requests[0]["idempotency_key"] != "axlr:settled:s:0:1" || link["rel"] != "supersedes" || link["ref"] != "project:probe:entry:decision:old" || requests[1]["continuation"] != "review-1" {
		t.Fatalf("%+v %+v", requests[0], requests[1])
	}
}
