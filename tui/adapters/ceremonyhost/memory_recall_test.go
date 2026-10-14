package ceremonyhost

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// recallAnswers replays KMP's packets as kmp-mcp 0.25.0 shaped them on 13
// October 2026 (probe_call.py on the lab store), recording each request.
func recallAnswers(t *testing.T, requests *[]map[string]any, tools *[]string, answers ...map[string]any) Memory {
	t.Helper()
	return Memory{Now: func() time.Time { return time.Date(2026, 10, 13, 17, 0, 0, 0, time.UTC) }, Tools: memoryToolsFunc(func(_ context.Context, id domain.ToolIdentity, args root.JSONValue) (domain.ToolOutcome, error) {
		var request map[string]any
		if err := json.Unmarshal(args.Bytes(), &request); err != nil {
			t.Fatal(err)
		}
		*requests = append(*requests, request)
		*tools = append(*tools, string(id.Plugin.ToolName))
		answer := answers[len(*requests)-1]
		if refusal, ok := answer["error"]; ok {
			packet, _ := json.Marshal(map[string]any{"output": map[string]any{"is_error": true, "structured_content": map[string]any{"error": refusal}}})
			return domain.ToolOutcome{Content: root.Text(packet), IsError: true}, nil
		}
		packet, _ := json.Marshal(map[string]any{"output": map[string]any{"structured_content": answer}})
		return domain.ToolOutcome{Content: root.Text(packet)}, nil
	})}
}

func TestRecallWakeTreatsAMissingAboutAsNothingRecorded(t *testing.T) {
	var requests []map[string]any
	var tools []string
	m := recallAnswers(t, &requests, &tools,
		map[string]any{"error": map[string]any{"code": "not_found", "message": "embedded kernel wake failed for `user:nobody`: node 'user:nobody' not found"}},
		map[string]any{"wake": map[string]any{"current_state": []any{"project:AXLR:entry:decision:x (decision): keep dates in UTC"}, "open_loops": []any{}, "next_actions": []any{"add the parser tests"}}})
	got, err := m.Recall(context.Background(), application.RecallRequest{About: "user:nobody", ContextID: "ctx_1", Intent: "resume the parser work"})
	if err != nil || !got.Empty || got.Mode != application.RecallWake {
		t.Fatalf("%+v %v", got, err)
	}
	if tools[0] != "kmp_wake" || requests[0]["context_id"] != "ctx_1" || requests[0]["intent"] != "resume the parser work" || requests[0]["about"] != "user:nobody" {
		t.Fatalf("%v %+v", tools, requests[0])
	}
	got, err = m.Recall(context.Background(), application.RecallRequest{Mode: application.RecallWake, About: "project:AXLR"})
	if err != nil || got.Empty || got.Text != "- project:AXLR:entry:decision:x (decision): keep dates in UTC\n- add the parser tests" || len(got.Refs) != 1 {
		t.Fatalf("%+v %v", got, err)
	}
	if _, has := requests[1]["context_id"]; has {
		t.Fatalf("no context sent for none: %+v", requests[1])
	}
}

func TestRecallAskCompactsTheEvidence(t *testing.T) {
	var requests []map[string]any
	var tools []string
	m := recallAnswers(t, &requests, &tools, map[string]any{
		"answer": "Retrieved for this question by term overlap; read proof.evidence and judge whether it answers: ws:s:entry:turn:t1", "answer_status": "answered", "summary": "Retrieved 1 memory item for: what did the session do",
		"because":    []any{map[string]any{"claim": "ws:s:entry:turn:t1", "ref": "detail:evidence:ws:s:entry:turn:t1:current"}},
		"proof":      map[string]any{"confidence": "high", "nearest_outside": nil, "evidence": []any{map[string]any{"id": "detail:evidence:ws:s:entry:turn:t1:current", "supports": []any{"ws:s:entry:turn:t1"}, "text": "MADE axlr-session-s step turn #1\naxlr_call_tool kmp_ask — why: ask", "time": "2026-10-13T15:22:19.097992491Z"}}},
		"warnings":   []any{"open-ended"},
		"projection": map[string]any{"page": map[string]any{"has_more": true}},
	})
	got, err := m.Recall(context.Background(), application.RecallRequest{Mode: application.RecallAsk, About: "ws:s", Intent: "what did the session do", Since: "2026-10-01T00:00:00Z", ContextID: "ctx_1"})
	if err != nil || got.Empty || tools[0] != "kmp_ask" {
		t.Fatalf("%+v %v %v", got, err, tools)
	}
	if requests[0]["question"] != "what did the session do" || requests[0]["context_id"] != "ctx_1" || requests[0]["interval"].(map[string]any)["start"] != "2026-10-01T00:00:00Z" || requests[0]["budget"].(map[string]any)["max_bytes"] != float64(recallAskBytes) {
		t.Fatalf("%+v", requests[0])
	}
	evidence := got.Answer["evidence"].([]map[string]any)
	if got.Answer["answer_status"] != "answered" || got.Answer["confidence"] != "high" || got.Answer["partial"] != true || len(evidence) != 1 || evidence[0]["ref"] != "ws:s:entry:turn:t1" || evidence[0]["time"] != "2026-10-13T15:22:19.097992491Z" || got.Answer["warnings"].([]any)[0] != "open-ended" {
		t.Fatalf("%+v", got.Answer)
	}
	if _, has := got.Answer["nearest_outside"]; has {
		t.Fatal("a null nearest_outside is not reported")
	}
	if _, err := m.Recall(context.Background(), application.RecallRequest{Mode: application.RecallAsk, About: "ws:s"}); err == nil {
		t.Fatal("ask without a question")
	}
}

func TestRecallHistoryRewindsFromNowOrReadsForwardFromSince(t *testing.T) {
	var requests []map[string]any
	var tools []string
	entries := map[string]any{"summary": "Returned 1 temporal entry.", "warnings": []any{}, "page": map[string]any{"has_more": false},
		"entries": []any{map[string]any{"kind": "turn", "ref": "ws:s:entry:turn:t1", "text": "Turn 1: no file, command or memory.", "coordinates": []any{map[string]any{"dimension": "mode", "occurred_at": "2026-10-13T15:22:19Z"}}}}}
	m := recallAnswers(t, &requests, &tools, entries, entries)
	got, err := m.Recall(context.Background(), application.RecallRequest{Mode: application.RecallHistory, About: "ws:s", Entries: 3})
	if err != nil || tools[0] != "kmp_time" || requests[0]["move"] != "rewind" || requests[0]["from"].(map[string]any)["time"] != "2026-10-13T17:00:00Z" || requests[0]["limit"].(map[string]any)["entries"] != float64(3) {
		t.Fatalf("%+v %v %+v", got, err, requests[0])
	}
	history := got.Answer["entries"].([]map[string]any)
	if len(history) != 1 || history[0]["ref"] != "ws:s:entry:turn:t1" || history[0]["occurred_at"] != "2026-10-13T15:22:19Z" || history[0]["kind"] != "turn" || got.Answer["summary"] != "Returned 1 temporal entry." {
		t.Fatalf("%+v", got.Answer)
	}
	if _, err := m.Recall(context.Background(), application.RecallRequest{Mode: application.RecallHistory, About: "ws:s", Since: "2026-10-01T00:00:00Z"}); err != nil || requests[1]["move"] != "forward" || requests[1]["interval"].(map[string]any)["start"] != "2026-10-01T00:00:00Z" || requests[1]["limit"].(map[string]any)["entries"] != float64(10) {
		t.Fatalf("%v %+v", err, requests[1])
	}
	if _, has := requests[1]["from"]; has {
		t.Fatalf("from with interval: %+v", requests[1])
	}
}
