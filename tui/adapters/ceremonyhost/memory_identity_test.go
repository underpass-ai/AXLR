package ceremonyhost

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// guideAnswers replays kmp_guide as kmp-mcp 0.25.0 answers it (13 October
// 2026): a registration key gives the agent and its first context; an
// agent id with a context key gives a new context. missing counts how many
// calls refuse for a store without the guide first.
func guideAnswers(t *testing.T, requests *[]map[string]any, missing int) Memory {
	t.Helper()
	return Memory{Tools: memoryToolsFunc(func(_ context.Context, id domain.ToolIdentity, args root.JSONValue) (domain.ToolOutcome, error) {
		if id.Plugin.PluginID != "kmp" || id.Plugin.ToolName != "kmp_guide" {
			t.Fatalf("called %+v", id)
		}
		var request map[string]any
		if err := json.Unmarshal(args.Bytes(), &request); err != nil {
			t.Fatal(err)
		}
		*requests = append(*requests, request)
		if len(*requests) <= missing {
			return domain.ToolOutcome{IsError: true, Content: `{"output":{"is_error":true,"structured_content":{"error":{"code":"invalid_request","message":"The selected store cannot serve guide node ` + "`guide:kmp-agent:overview`" + `. Run ` + "`kmp-mcp guide sync --plugin-root <dir>`" + ` for this store."}}}}`}, nil
		}
		var answer map[string]any
		if key, _ := request["registration_key"].(string); key != "" {
			answer = map[string]any{"agent": map[string]any{"id": "agent_7", "name": key}, "context_id": "ctx_registration", "guide_revision": "r1"}
		} else {
			answer = map[string]any{"agent": map[string]any{"id": request["agent_id"]}, "context_id": "ctx_" + request["context_key"].(string), "guide_revision": "r1"}
		}
		packet, _ := json.Marshal(map[string]any{"output": map[string]any{"structured_content": answer}})
		return domain.ToolOutcome{Content: root.Text(packet)}, nil
	})}
}

func TestGuideRegistersAnAgentAndOpensContexts(t *testing.T) {
	var requests []map[string]any
	m := guideAnswers(t, &requests, 0)
	got, err := m.Guide(context.Background(), "axlr:install-1", "", "")
	if err != nil || got.AgentID != "agent_7" || got.ContextID != "ctx_registration" {
		t.Fatalf("%+v %v", got, err)
	}
	got, err = m.Guide(context.Background(), "", "agent_7", "axlr:session:s1")
	if err != nil || got.AgentID != "agent_7" || got.ContextID != "ctx_axlr:session:s1" {
		t.Fatalf("%+v %v", got, err)
	}
	if requests[0]["registration_key"] != "axlr:install-1" || requests[0]["agent_id"] != nil || requests[1]["agent_id"] != "agent_7" || requests[1]["context_key"] != "axlr:session:s1" || requests[1]["registration_key"] != nil {
		t.Fatalf("requests %+v", requests)
	}
}

func TestGuideSyncsAMissingGuideOnceAndTriesAgain(t *testing.T) {
	var requests []map[string]any
	synced := 0
	m := guideAnswers(t, &requests, 1)
	m.SyncGuide = func(context.Context) error { synced++; return nil }
	got, err := m.Guide(context.Background(), "axlr:install-1", "", "")
	if err != nil || got.AgentID != "agent_7" || synced != 1 || len(requests) != 2 {
		t.Fatalf("%+v %v synced=%d calls=%d", got, err, synced, len(requests))
	}
	// A sync that fails reports both.
	requests = nil
	m = guideAnswers(t, &requests, 2)
	m.SyncGuide = func(context.Context) error { return errors.New("no plugin root") }
	if _, err := m.Guide(context.Background(), "axlr:install-1", "", ""); err == nil || len(requests) != 1 {
		t.Fatalf("%v calls=%d", err, len(requests))
	}
	// Without a sync the refusal is returned as it is.
	requests = nil
	m = guideAnswers(t, &requests, 2)
	var refused *refusal
	if _, err := m.Guide(context.Background(), "axlr:install-1", "", ""); !errors.As(err, &refused) || len(requests) != 1 {
		t.Fatalf("%v calls=%d", err, len(requests))
	}
}

func TestWritesCarryTheSessionContext(t *testing.T) {
	var requests []map[string]any
	m := kmpAnswers(t, &requests, map[string]any{"accepted": true, "status": "committed", "local_refs": map[string]any{"m1": "ws:s1:entry:turn:x"}}, map[string]any{"accepted": true, "status": "committed", "local_refs": map[string]any{"m1": "project:probe:entry:constraint:y"}})
	entry := application.TurnEntry{About: "ws:s1", Key: "axlr:turn:s1:0", Summary: "Turn 1: 1 file written (a.go).", Evidence: []string{"MADE axlr-session-s1 step turn #1"}, OccurredAt: time.Now(), ContextID: "ctx_s1"}
	if _, err := m.RecordTurn(context.Background(), entry); err != nil {
		t.Fatal(err)
	}
	request := rememberRequest()
	request.ContextID = "ctx_s1"
	if _, err := m.Remember(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if requests[0]["context_id"] != "ctx_s1" || requests[1]["context_id"] != "ctx_s1" {
		t.Fatalf("requests %+v", requests)
	}
	// Without a context nothing is sent for it.
	requests = nil
	m = kmpAnswers(t, &requests, map[string]any{"accepted": true, "status": "committed", "local_refs": map[string]any{"m1": "project:probe:entry:constraint:y"}})
	if _, err := m.Remember(context.Background(), rememberRequest()); err != nil {
		t.Fatal(err)
	}
	if _, has := requests[0]["context_id"]; has {
		t.Fatalf("request %+v", requests[0])
	}
}

func TestRecordSettledWritesTheSelectedMemoryAndResumesItsReview(t *testing.T) {
	var requests []map[string]any
	m := kmpAnswers(t, &requests,
		map[string]any{"accepted": false, "status": "needs_review", "continuation": "review-1"},
		map[string]any{"accepted": true, "status": "committed", "local_refs": map[string]any{"settled": "project:probe:entry:decision:utc"}})
	entry := application.SettledEntry{About: "project:probe", Kind: "decision", Text: "Las fechas se guardan en UTC.", SummaryEN: "decision settled in turn 1: \"Las fechas se guardan en UTC.\"", Evidence: []string{"MADE i step turn #1", "local_write parser.go"}, Labels: map[string][]string{"judged_by": {"jev"}}, Key: "axlr:settled:s:0:1", ContextID: "ctx_1", Links: []application.MemoryLink{{Ref: "project:probe:entry:decision:old", Rel: "supersedes", Why: "Jev judged (p=0.60) that the turn's result replaces it", Evidence: "local_write parser.go"}}}
	ref, err := m.RecordSettled(context.Background(), entry)
	if err != nil || ref != "project:probe:entry:decision:utc" || len(requests) != 2 {
		t.Fatalf("%q %v %d", ref, err, len(requests))
	}
	memory := requests[0]["memories"].([]any)[0].(map[string]any)
	link := memory["connect_to"].([]any)[0].(map[string]any)
	if memory["kind"] != "decision" || memory["summary"] != entry.Text || memory["summary_en"] != entry.SummaryEN || requests[0]["context_id"] != "ctx_1" || requests[0]["idempotency_key"] != "axlr:settled:s:0:1" || link["rel"] != "supersedes" || link["ref"] != "project:probe:entry:decision:old" || requests[1]["continuation"] != "review-1" {
		t.Fatalf("%+v %+v", requests[0], requests[1])
	}
}
