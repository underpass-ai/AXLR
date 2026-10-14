package ceremonyhost

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func packet(t *testing.T, content map[string]any) domain.ToolOutcome {
	t.Helper()
	data, err := json.Marshal(map[string]any{"output": map[string]any{"structured_content": content}})
	if err != nil {
		t.Fatal(err)
	}
	return domain.ToolOutcome{Content: root.Text(data)}
}

// /memory compare asks kmp_relate for the abouts in the session's context
// and reads the declared relations, the proposals and the facts.
func TestCompareReadsRelateBetweenAbouts(t *testing.T) {
	var request map[string]any
	m := Memory{Tools: memoryToolsFunc(func(_ context.Context, id domain.ToolIdentity, args root.JSONValue) (domain.ToolOutcome, error) {
		if id.Plugin.ToolName != "kmp_relate" {
			t.Fatalf("tool %s", id.Plugin.ToolName)
		}
		if err := json.Unmarshal(args.Bytes(), &request); err != nil {
			t.Fatal(err)
		}
		return packet(t, map[string]any{
			"declared":   []any{map[string]any{"from": "project:A:entry:decision:x", "to": "project:B:entry:decision:y", "rel": "same_entity_as", "why": "declared"}},
			"coordinate": []any{map[string]any{"from": "project:A:entry:decision:x", "to": "ws:1:entry:turn:t", "kind": "before", "dimension": "session", "scope_id": "1", "why": "Both stand in session=1"}, map[string]any{"from": "", "to": "x"}},
			"facts":      []any{map[string]any{"about": "project:A", "ref": "project:A:entry:decision:x"}, map[string]any{"about": "project:A", "ref": "r2"}, map[string]any{"about": "ws:1", "ref": "r3"}},
		}), nil
	})}
	result, err := m.Compare(context.Background(), application.CompareRequest{About: "project:A", Others: []string{"ws:1", "project:A", "project:B"}, ContextID: "ctx-1"})
	if err != nil {
		t.Fatal(err)
	}
	dimensions := request["dimensions"].(map[string]any)
	if request["about"] != "project:A" || request["context_id"] != "ctx-1" || dimensions["scope"] != "abouts" || len(dimensions["abouts"].([]any)) != 3 {
		t.Fatalf("request %+v", request)
	}
	if strings.Join(result.Abouts, ",") != "project:A,ws:1,project:B" || len(result.Declared) != 1 || len(result.Proposals) != 1 || result.Facts["project:A"] != 2 || result.Facts["ws:1"] != 1 {
		t.Fatalf("result %+v", result)
	}
	if p := result.Proposals[0]; p.Rel != "before" || p.Dimension != "session" || p.Scope != "1" || p.Why != "Both stand in session=1" {
		t.Fatalf("proposal %+v", p)
	}
	if _, err := m.Compare(context.Background(), application.CompareRequest{About: "project:A", Others: []string{"project:A"}}); err == nil {
		t.Fatal("one about compared with itself")
	}
}

// /memory forget inspects the entry under its own about, writes an
// observation that supersedes it with the person's reason as evidence,
// and commits KMP's review with the continuation; the entry is never
// removed.
func TestForgetSupersedesTheEntryAndCommitsTheReview(t *testing.T) {
	var tools []string
	var write map[string]any
	ref := "project:Repo-B:entry:decision:las-fechas-166c94f5031adc46"
	m := Memory{Now: func() time.Time { return time.Date(2026, 10, 13, 21, 0, 0, 0, time.UTC) }, Tools: memoryToolsFunc(func(_ context.Context, id domain.ToolIdentity, args root.JSONValue) (domain.ToolOutcome, error) {
		tools = append(tools, string(id.Plugin.ToolName))
		var request map[string]any
		if err := json.Unmarshal(args.Bytes(), &request); err != nil {
			t.Fatal(err)
		}
		switch len(tools) {
		case 1:
			if id.Plugin.ToolName != "kmp_inspect" || request["about"] != "project:Repo-B" || request["ref"] != ref || request["context_id"] != "ctx-1" {
				t.Fatalf("inspect %+v", request)
			}
			return packet(t, map[string]any{"object": map[string]any{"kind": "decision", "text": "Las fechas en UTC.", "metadata": map[string]any{"summary_en": "Dates in UTC."}}}), nil
		case 2:
			write = request
			return packet(t, map[string]any{"accepted": false, "status": "needs_review", "local_refs": map[string]any{"forget": "project:Repo-B:entry:observation:forgotten-1"}, "next_actions": []any{map[string]any{"tool": "kmp_write_memory", "arguments": map[string]any{"continuation": "read_1"}}}}), nil
		default:
			if request["continuation"] != "read_1" || len(request) != 1 {
				t.Fatalf("resume %+v", request)
			}
			return packet(t, map[string]any{"accepted": true, "status": "committed"}), nil
		}
	})}
	receipt, err := m.Forget(context.Background(), application.ForgetRequest{Ref: ref, Why: "ya no aplica", Command: "/memory forget " + ref + " ya no aplica", ContextID: "ctx-1", Labels: map[string][]string{"session": {"s1"}}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(tools, ",") != "kmp_inspect,kmp_write_memory,kmp_write_memory" {
		t.Fatalf("tools %v", tools)
	}
	memory := write["memories"].([]any)[0].(map[string]any)
	link := memory["connect_to"].([]any)[0].(map[string]any)
	if write["about"] != "project:Repo-B" || write["actor"] != "axlr" || !strings.HasPrefix(write["idempotency_key"].(string), "axlr:forget:") || write["labels"].(map[string]any)["session"].([]any)[0] != "s1" {
		t.Fatalf("write %+v", write)
	}
	if memory["kind"] != "observation" || memory["summary"] != "Forgotten at the person's request: Las fechas en UTC." || memory["summary_en"] != "Forgotten at the person's request: Dates in UTC." || memory["evidence"] != "/memory forget "+ref+" ya no aplica on 2026-10-13T21:00:00Z: ya no aplica" {
		t.Fatalf("memory %+v", memory)
	}
	if link["ref"] != ref || link["rel"] != "supersedes" || link["why"] != "ya no aplica" || link["confidence"] != "high" {
		t.Fatalf("link %+v", link)
	}
	if receipt.About != "project:Repo-B" || receipt.Text != "Las fechas en UTC." || receipt.Superseding != "project:Repo-B:entry:observation:forgotten-1" {
		t.Fatalf("receipt %+v", receipt)
	}
	if _, err := m.Forget(context.Background(), application.ForgetRequest{Ref: "no-entry"}); err == nil {
		t.Fatal("a ref without an about accepted")
	}
}
