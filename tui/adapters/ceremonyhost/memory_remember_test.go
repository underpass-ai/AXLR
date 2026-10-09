package ceremonyhost

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// kmpAnswers replays kmp_write_memory answers shaped as kmp-mcp 0.25.0 gave
// them on an isolated store (9 October 2026), recording each request.
func kmpAnswers(t *testing.T, requests *[]map[string]any, answers ...map[string]any) Memory {
	t.Helper()
	return Memory{Tools: memoryToolsFunc(func(_ context.Context, id domain.ToolIdentity, args root.JSONValue) (domain.ToolOutcome, error) {
		if id.Plugin.PluginID != "kmp" || id.Plugin.ToolName != "kmp_write_memory" {
			t.Fatalf("called %+v", id)
		}
		var request map[string]any
		if err := json.Unmarshal(args.Bytes(), &request); err != nil {
			t.Fatal(err)
		}
		*requests = append(*requests, request)
		answer := answers[len(*requests)-1]
		isError := answer["status"] == "rejected"
		packet, _ := json.Marshal(map[string]any{"output": map[string]any{"is_error": isError, "structured_content": answer}})
		return domain.ToolOutcome{Content: root.Text(packet), IsError: isError}, nil
	})}
}

func rememberRequest() application.RememberRequest {
	return application.RememberRequest{About: "project:probe", Kind: "constraint", Text: "Payload capture stays opt-in.", Evidence: []string{"trace_payloads default false", "docs/console.md"},
		Labels: map[string][]string{"session": {"s1"}, "ws": {"/tmp/ws"}}, IdempotencyKey: "axlr:remember:0011223344556677"}
}

func TestRememberWritesOneMemoryAndAnswersWithItsRef(t *testing.T) {
	var requests []map[string]any
	m := kmpAnswers(t, &requests, map[string]any{"accepted": true, "status": "committed", "local_refs": map[string]any{"m1": "project:probe:entry:constraint:payload-capture-stays-opt-in-112ff7e3b0f6ba09"},
		"viewer": map[string]any{"url": "http://127.0.0.1:39807/?k=x"}, "clocks": map[string]any{"entries": 1}})
	got, err := m.Remember(context.Background(), rememberRequest())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"accepted": true, "status": "committed", "ref": "project:probe:entry:constraint:payload-capture-stays-opt-in-112ff7e3b0f6ba09", "about": "project:probe", "idempotency_key": "axlr:remember:0011223344556677"}
	if encoded, _ := json.Marshal(got); string(encoded) != mustJSON(t, want) {
		t.Fatalf("answer %s", encoded)
	}
	request := requests[0]
	memory := request["memories"].([]any)[0].(map[string]any)
	if request["about"] != "project:probe" || request["actor"] != "axlr" || request["idempotency_key"] != "axlr:remember:0011223344556677" || request["labels"] == nil ||
		memory["kind"] != "constraint" || memory["summary"] != "Payload capture stays opt-in." || memory["evidence"] != "trace_payloads default false\ndocs/console.md" || memory["connect_to"] != nil {
		t.Fatalf("request %+v", request)
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

// A write with links comes back for review: the answer keeps the links,
// the stored context they touch, bounded, and the continuation; sending the
// continuation commits, and the receipt names the about and the key.
func TestRememberReturnsKMPsReviewCompactAndCommitsItsContinuation(t *testing.T) {
	var requests []map[string]any
	long := strings.Repeat("stored context ", 40)
	items := []any{}
	for i := 0; i < 10; i++ {
		items = append(items, map[string]any{"ref": "project:probe:entry:decision:x", "kind": "decision", "state": "stored", "text": long, "action": map[string]any{"tool": "kmp_inspect"}, "clocks": map[string]any{}})
	}
	m := kmpAnswers(t, &requests,
		map[string]any{"accepted": false, "status": "needs_review", "relations": []any{map[string]any{"from": "@m1", "rel": "supports", "to": "project:probe:entry:decision:x"}},
			"neighborhood": map[string]any{"items": items, "token": "07d3"}, "next_actions": []any{map[string]any{"tool": "kmp_write_memory", "arguments": map[string]any{"continuation": "read_78fb"}}},
			"expand_context": []any{map[string]any{"tool": "kmp_wake"}}, "summary": "Nothing written."},
		map[string]any{"accepted": true, "status": "committed", "local_refs": map[string]any{"m4": "project:probe:entry:constraint:y"}, "receipt": map[string]any{"ref": "receipt:v1:project%3Aprobe:axlr%3Aremember%3A0011223344556677"}})
	request := rememberRequest()
	request.Links = []application.MemoryLink{{Ref: "project:probe:entry:decision:x", Rel: "supports", Why: "same release"}}
	review, err := m.Remember(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	link := requests[0]["memories"].([]any)[0].(map[string]any)["connect_to"].([]any)[0].(map[string]any)
	if link["evidence"] != "trace_payloads default false" || link["rel"] != "supports" || link["why"] != "same release" {
		t.Fatalf("link %+v", link)
	}
	stored := review["context"].([]any)
	if review["needs_review"] != true || review["accepted"] != false || review["continuation"] != "read_78fb" || len(stored) != rememberContextItems || review["context_omitted"] != 2 || review["links"] == nil {
		t.Fatalf("review %+v", review)
	}
	if text := stored[0].(map[string]any)["text"].(string); len(text) > rememberTextBytes || strings.Contains(mustJSON(t, review), "kmp_inspect") {
		t.Fatalf("review not compact: %s", mustJSON(t, review))
	}
	committed, err := m.Remember(context.Background(), application.RememberRequest{Continuation: "read_78fb"})
	if err != nil {
		t.Fatal(err)
	}
	if mustJSON(t, requests[1]) != `{"continuation":"read_78fb"}` || committed["accepted"] != true || committed["about"] != "project:probe" || committed["idempotency_key"] != "axlr:remember:0011223344556677" {
		t.Fatalf("commit %+v after %v", committed, requests[1])
	}
}

func TestRememberNamesWhatKMPRefused(t *testing.T) {
	var requests []map[string]any
	m := kmpAnswers(t, &requests, map[string]any{"status": "rejected", "error": map[string]any{"code": "invalid_argument", "message": "unsupported or vague kmp_write_memory relation `refines`"}})
	_, err := m.Remember(context.Background(), rememberRequest())
	if err == nil || !strings.Contains(err.Error(), "KMP refused the memory (invalid_argument): unsupported or vague kmp_write_memory relation `refines`") {
		t.Fatalf("err = %v", err)
	}
}
