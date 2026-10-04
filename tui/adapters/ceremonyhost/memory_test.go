package ceremonyhost

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestWakeProseKeepsOnlyWhatAModelCanUse(t *testing.T) {
	packet := map[string]any{
		"projection": map[string]any{"budget": map[string]any{"max_bytes": 12000}},
		"summary":    "Objective: ws:abc — Memory anchor",
		"wake": map[string]any{
			"current_state": []any{"ws:abc:entry:decision:x (decision): axlr_debug 2.0 ended COMPLETED. Fixed split().", "…"},
			"open_loops":    []any{},
			"next_actions":  []any{"Normalise punctuation"},
		},
	}
	got := wakeProse(packet)
	if got != "- ws:abc:entry:decision:x (decision): axlr_debug 2.0 ended COMPLETED. Fixed split().\n- Normalise punctuation" {
		t.Fatalf("got %q", got)
	}
	if strings.Contains(got, "projection") || wakeProse(map[string]any{}) != "" {
		t.Fatal("envelope leaked or empty packet produced text")
	}
}

func TestWakeProsePreservesPendingReadAndReference(t *testing.T) {
	got := wakeProse(map[string]any{
		"projection": map[string]any{"page": map[string]any{"has_more": true}, "next_action": map[string]any{"tool": "kmp_wake", "arguments": map[string]any{"continuation": "opaque-page"}}},
		"wake":       map[string]any{"current_state": []any{"project:AXLR:entry:x (observation): retained evidence"}},
	})
	for _, want := range []string{"Partial KMP recall", `"continuation":"opaque-page"`, "project:AXLR:entry:x"} {
		if !strings.Contains(got, want) {
			t.Fatalf("lost recall qualification %s: %s", want, got)
		}
	}
}

type memoryToolsFunc func(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error)

func (f memoryToolsFunc) Execute(ctx context.Context, id domain.ToolIdentity, args root.JSONValue) (domain.ToolOutcome, error) {
	return f(ctx, id, args)
}

func TestRecordHasStableWriteIdentityAndDoesNotBlindlyResumeReview(t *testing.T) {
	for _, status := range []string{"committed", "replayed", "needs_review", "unconfirmed", "rejected"} {
		t.Run(status, func(t *testing.T) {
			calls := 0
			m := Memory{Tools: memoryToolsFunc(func(_ context.Context, id domain.ToolIdentity, args root.JSONValue) (domain.ToolOutcome, error) {
				calls++
				var request map[string]any
				if err := json.Unmarshal(args.Bytes(), &request); err != nil {
					t.Fatal(err)
				}
				if id.Plugin.PluginID != "kmp" || id.Plugin.ToolName != "kmp_write_memory" || request["idempotency_key"] != "axlr:ceremony-outcome:instance-1" || request["about"] != "project:AXLR" || request["continuation"] != nil {
					t.Fatalf("bad write identity: %+v %+v", id, request)
				}
				memory := request["memories"].([]any)[0].(map[string]any)
				if memory["kind"] != "observation" || memory["evidence"] != "MADE instance instance-1" {
					t.Fatalf("unfaithful outcome: %+v", memory)
				}
				accepted := status == "committed" || status == "replayed"
				packet, _ := json.Marshal(map[string]any{"output": map[string]any{"structured_content": map[string]any{"status": status, "accepted": accepted, "continuation": "review-that-must-be-read"}}})
				return domain.ToolOutcome{Content: root.Text(packet)}, nil
			})}
			err := m.Record(context.Background(), "project:AXLR", map[string][]string{"ceremony": {"axlr_debug"}}, "instance-1", "Debug completed.", "MADE instance instance-1")
			if (err == nil) != (status == "committed" || status == "replayed") || calls != 1 {
				t.Fatalf("status=%s calls=%d err=%v", status, calls, err)
			}
		})
	}
}

func TestMissingKnownProjectIsNotSilentlyTreatedAsNewSession(t *testing.T) {
	m := Memory{Tools: memoryToolsFunc(func(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error) {
		return domain.ToolOutcome{IsError: true, Content: `{"output":{"is_error":true,"structured_content":{"error":{"code":"not_found","message":"node not found"}}}}`}, nil
	})}
	if _, err := m.Wake(context.Background(), "project:AXLR"); err == nil {
		t.Fatal("missing known scope was masked")
	}
	if got, err := m.Wake(context.Background(), "ws:new-session"); err != nil || got != "" {
		t.Fatalf("new session scope: %q %v", got, err)
	}
}
