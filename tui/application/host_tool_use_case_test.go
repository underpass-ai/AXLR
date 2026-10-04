package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
	"strings"
	"testing"
	"unicode/utf8"
)

func hostSession(t *testing.T, text string, plugins ...domain.AvailableTool) domain.Session {
	t.Helper()
	s := turnSession(t)
	tools := append(turnTools(), HostTools()...)
	tools = append(tools, plugins...)
	if err := s.BeginTurn("user instruction", tools); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAssistant(assistant(root.Text(text))); err != nil {
		t.Fatal(err)
	}
	restored, err := domain.RestoreSession(s.Export())
	if err != nil {
		t.Fatal(err)
	}
	return restored
}

func hostExecute(t *testing.T, s domain.Session, op, args string) domain.ToolOutcome {
	t.Helper()
	id, err := domain.NewHostToolIdentity(op)
	if err != nil {
		t.Fatal(err)
	}
	out, err := (HostToolUseCase{}).Execute(context.Background(), s, id, hostJSON(t, args))
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Content) > MaxHostResultBytes || !json.Valid([]byte(out.Content)) || out.Uncertain {
		t.Fatalf("invalid bounded read-only result: %+v", out)
	}
	return out
}

func TestHostDiscoveryListsStableSummariesAndExactSchema(t *testing.T) {
	s := hostSession(t, "reply", hostPlugin(t, "zeta", "made", "run"), hostPlugin(t, "alpha", "kmp", "kmp_wake"))
	out := hostExecute(t, s, domain.HostOperationTools, `{"limit":1}`)
	var result struct {
		Tools        []struct{ Name string }
		TotalMatches int  `json:"total_matches"`
		HasMore      bool `json:"has_more"`
	}
	if err := json.Unmarshal([]byte(out.Content), &result); err != nil || out.IsError || len(result.Tools) != 1 || result.Tools[0].Name != "alpha" || result.TotalMatches != 2 || !result.HasMore {
		t.Fatalf("%s %v", out.Content, err)
	}
	out = hostExecute(t, s, domain.HostOperationTools, `{"query":"KMP WAKE"}`)
	if out.IsError || !strings.Contains(string(out.Content), `"name":"alpha"`) || strings.Contains(string(out.Content), `"name":"zeta"`) || strings.Contains(string(out.Content), `"properties"`) {
		t.Fatalf("%s", out.Content)
	}
	out = hostExecute(t, s, domain.HostOperationTools, `{"name":"alpha"}`)
	if out.IsError || !strings.Contains(string(out.Content), `"parameters":{`) || !strings.Contains(string(out.Content), `"type":"object"`) || !strings.Contains(string(out.Content), `"plugin":"kmp"`) {
		t.Fatalf("%s", out.Content)
	}
	out = hostExecute(t, s, domain.HostOperationTools, `{"query":"no such capability"}`)
	if out.IsError || !strings.Contains(string(out.Content), `"total_matches":0`) {
		t.Fatalf("%s", out.Content)
	}
}

func TestHostDiscoveryRejectsInvalidRangesAndSchemaWithoutTruncation(t *testing.T) {
	s := hostSession(t, "reply", hostPlugin(t, "memory", "kmp", "kmp_wake"))
	for _, args := range []string{`{"name":"read"}`, `{"name":"unknown"}`, `{"name":null}`, `{"name":"memory","query":"wake"}`, `{"query":3}`, `{"query":null}`, `{"limit":0}`, `{"limit":21}`, `{"limit":1.5}`, `{"limit":null}`, `{"query":"x","query":"y"}`, `{"extra":true}`, `{"offset":-1}`, `{"offset":null}`, `{"offset":1.5}`, `{"offset":2}`, `{"name":"memory","offset":0}`} {
		if out := hostExecute(t, s, domain.HostOperationTools, args); !out.IsError {
			t.Fatalf("accepted %s: %s", args, out.Content)
		}
	}
	query, _ := json.Marshal(map[string]string{"query": strings.Repeat("x", 513)})
	if out := hostExecute(t, s, domain.HostOperationTools, string(query)); !out.IsError {
		t.Fatal("unbounded query")
	}
	huge := hostPlugin(t, "huge", "kmp", "kmp_guide")
	schema, _ := json.Marshal(map[string]any{"type": "object", "description": strings.Repeat("z", MaxHostResultBytes)})
	huge.Definition.Parameters = hostJSON(t, string(schema))
	s = hostSession(t, "reply", huge)
	out := hostExecute(t, s, domain.HostOperationTools, `{"name":"huge"}`)
	if out.IsError || !strings.Contains(string(out.Content), `"outline"`) || !strings.Contains(string(out.Content), `"hint"`) || strings.Contains(string(out.Content), `"parameters"`) || len(out.Content) > MaxHostResultBytes {
		t.Fatalf("oversized schema was not outlined: %.300s", out.Content)
	}
	if out := hostExecute(t, s, domain.HostOperationTools, `{"name":"huge","path":"/description"}`); !out.IsError || !strings.Contains(string(out.Content), "exceeds") {
		t.Fatalf("oversized leaf silently cut: %.300s", out.Content)
	}
	for _, args := range []string{`{"path":"/type"}`, `{"name":"huge","path":"type"}`, `{"name":"huge","path":"/missing"}`, `{"name":"huge","path":3}`} {
		if out := hostExecute(t, s, domain.HostOperationTools, args); !out.IsError {
			t.Fatalf("accepted %s", args)
		}
	}
}

func TestHostDiscoveryStripsAnnotationsAndSelectsPaths(t *testing.T) {
	tool := hostPlugin(t, "design", "made", "made_design_ceremony")
	schema, _ := json.Marshal(map[string]any{
		"type":                   "object",
		"x-made-pattern-catalog": strings.Repeat("catalogue ", 8000),
		"x-made-shape":           "one of two shapes",
		"properties": map[string]any{
			"x-literal": map[string]any{"type": "string"},
			"stages":    map[string]any{"type": "array", "items": map[string]any{"type": "object", "x-made-shape": "a stage", "properties": map[string]any{"id": map[string]any{"type": "string"}}}},
		},
	})
	tool.Definition.Parameters = hostJSON(t, string(schema))
	s := hostSession(t, "reply", tool)
	out := hostExecute(t, s, domain.HostOperationTools, `{"name":"design"}`)
	content := string(out.Content)
	if out.IsError || strings.Contains(content, "catalogue") || strings.Contains(content, `"x-made-shape":`) || !strings.Contains(content, `"x-literal"`) || !strings.Contains(content, `"annotations_omitted":["x-made-pattern-catalog","x-made-shape"]`) {
		t.Fatalf("annotations not stripped or property lost: %.400s", content)
	}
	out = hostExecute(t, s, domain.HostOperationTools, `{"name":"design","path":"/properties/stages/items"}`)
	if out.IsError || !strings.Contains(string(out.Content), `"path":"/properties/stages/items"`) || !strings.Contains(string(out.Content), `"id":{"type":"string"}`) {
		t.Fatalf("path did not select the subschema: %s", out.Content)
	}
}

func TestHostHistoryRecoversUnicodeMessageByBoundedPages(t *testing.T) {
	s := hostSession(t, strings.Repeat("historia 😊\n", 3000))
	expected := []byte(s.Messages()[1].Content)
	var recovered strings.Builder
	offset := 0
	for {
		args, _ := json.Marshal(map[string]int{"message_index": 1, "offset_bytes": offset, "limit_bytes": 97})
		out := hostExecute(t, s, domain.HostOperationHistory, string(args))
		if out.IsError {
			t.Fatal(out.Content)
		}
		var page struct {
			Text  string
			Next  int  `json:"next_offset_bytes"`
			Total int  `json:"total_bytes"`
			More  bool `json:"has_more"`
		}
		if err := json.Unmarshal([]byte(out.Content), &page); err != nil {
			t.Fatal(err)
		}
		if !utf8.ValidString(page.Text) || len(page.Text) > 97 || page.Total != len(expected) || page.Next <= offset {
			t.Fatalf("bad page: %+v", page)
		}
		recovered.WriteString(page.Text)
		offset = page.Next
		if !page.More {
			break
		}
	}
	if recovered.String() != string(expected) {
		t.Fatal("paged history lost content")
	}
	args, _ := json.Marshal(map[string]int{"message_index": 1, "offset_bytes": len(expected)})
	if out := hostExecute(t, s, domain.HostOperationHistory, string(args)); out.IsError || !strings.Contains(string(out.Content), `"has_more":false`) {
		t.Fatal("EOF page")
	}
}

func TestHostHistoryRejectsBadRangesAndBoundsEscapedPayload(t *testing.T) {
	s := hostSession(t, "😊 reply")
	for _, args := range []string{`{}`, `{"message_index":-1}`, `{"message_index":9}`, `{"message_index":null}`, `{"message_index":1,"offset_bytes":-1}`, `{"message_index":1,"offset_bytes":999}`, `{"message_index":1,"limit_bytes":0}`, `{"message_index":1,"limit_bytes":32769}`, `{"message_index":1,"limit_bytes":null}`, `{"message_index":1,"limit_bytes":1.2}`, `{"message_index":1,"other":true}`} {
		if out := hostExecute(t, s, domain.HostOperationHistory, args); !out.IsError {
			t.Fatalf("accepted %s", args)
		}
	}
	data := string(s.Messages()[1].Content)
	start := strings.Index(data, "😊")
	for _, test := range []map[string]int{{"message_index": 1, "offset_bytes": start + 1}, {"message_index": 1, "offset_bytes": start, "limit_bytes": 1}} {
		args, _ := json.Marshal(test)
		if out := hostExecute(t, s, domain.HostOperationHistory, string(args)); !out.IsError {
			t.Fatal("UTF8 split accepted")
		}
	}
	s = hostSession(t, strings.Repeat("\"\\", 20000))
	out := hostExecute(t, s, domain.HostOperationHistory, `{"message_index":1,"limit_bytes":16384}`)
	if out.IsError || contentJSONBytes(string(out.Content)) > MaxHistoryReadBytes-256 {
		t.Fatal("escaped page exceeded model result cap")
	}
}

func TestHostHistoryPagesToolContentWithoutReescaping(t *testing.T) {
	result := `{"status":"completed","output":{"text":"` + strings.Repeat("x", 4000) + `"}}`
	messages := []root.Message{
		{Role: root.RoleUser, Content: "read"},
		{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "call_read", Name: "local_read", Arguments: hostJSON(t, `{"path":"a"}`)}}},
		{Role: root.RoleTool, ToolCallID: "call_read", Content: root.Text(result)},
		{Role: root.RoleUser, Content: "next"},
	}
	page, err := hostHistory(messages, hostJSON(t, `{"message_index":2,"limit_bytes":16384}`))
	if err != nil {
		t.Fatal(err)
	}
	fields := page.(map[string]any)
	if fields["text"] != result || fields["has_more"] != false || fields["total_bytes"] != len(result) || fields["tool_call_id"] != root.ToolCallID("call_read") {
		t.Fatalf("tool content was not returned verbatim in one page: %+v", fields)
	}
	page, err = hostHistory(messages, hostJSON(t, `{"message_index":1}`))
	if err != nil || page.(map[string]any)["tool_calls"] == nil {
		t.Fatalf("assistant tool calls missing: %v %+v", err, page)
	}
}

func TestHostExecutionHasNoWrapperPrivilegeOrUncertainEffects(t *testing.T) {
	s := hostSession(t, "reply")
	if out := hostExecute(t, s, domain.HostOperationCallTool, `{"name":"made_run","arguments":{}}`); !out.IsError {
		t.Fatal("host directly executed plugin wrapper")
	}
	local, _ := domain.NewLocalToolIdentity("exec")
	for _, id := range []domain.ToolIdentity{local, {}} {
		out, err := (HostToolUseCase{}).Execute(context.Background(), s, id, hostJSON(t, `{}`))
		if err != nil || !out.IsError || out.Uncertain {
			t.Fatal("non-host identity admitted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	id, _ := domain.NewHostToolIdentity(domain.HostOperationHistory)
	if _, err := (HostToolUseCase{}).Execute(ctx, s, id, hostJSON(t, `{}`)); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation ignored")
	}
	if len(hostFailure(errors.New(strings.Repeat("bad", 10000))).Content) > MaxHostResultBytes {
		t.Fatal("unbounded host error")
	}
}

func TestHostDiscoveryPagesEveryRegisteredTool(t *testing.T) {
	plugins := make([]domain.AvailableTool, 25)
	for i := range plugins {
		plugins[i] = hostPlugin(t, fmt.Sprintf("tool_%02d", i), "kmp", fmt.Sprintf("action_%02d", i))
	}
	s := hostSession(t, "reply", plugins...)
	offset := 0
	names := map[string]bool{}
	for {
		out := hostExecute(t, s, domain.HostOperationTools, fmt.Sprintf(`{"limit":7,"offset":%d}`, offset))
		var page struct {
			Tools []struct{ Name string }
			Next  int  `json:"next_offset"`
			More  bool `json:"has_more"`
		}
		if err := json.Unmarshal([]byte(out.Content), &page); err != nil || out.IsError {
			t.Fatal(out.Content, err)
		}
		for _, tool := range page.Tools {
			if names[tool.Name] {
				t.Fatal("duplicate tool")
			}
			names[tool.Name] = true
		}
		if !page.More {
			break
		}
		if page.Next <= offset {
			t.Fatal("nonadvancing pagination")
		}
		offset = page.Next
	}
	if len(names) != 25 {
		t.Fatalf("catalog incomplete: %d", len(names))
	}
}

func TestHistoryRecoveryPageSurvivesDefaultProjectionExactly(t *testing.T) {
	s := hostSession(t, strings.Repeat(`"\😊`, 10000))
	out := hostExecute(t, s, domain.HostOperationHistory, `{"message_index":1,"limit_bytes":16384}`)
	messages := []root.Message{{Role: root.RoleUser, Content: "recover page"}, {Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "history", Name: HostHistoryName, Arguments: hostJSON(t, `{"message_index":1}`)}}}, {Role: root.RoleTool, ToolCallID: "history", Content: out.Content}}
	projection, err := NewDefaultModelContextProjector().Project(messages)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Messages[2].Content != out.Content {
		t.Fatal("recovery page was truncated a second time")
	}
	var page struct {
		Next int  `json:"next_offset_bytes"`
		More bool `json:"has_more"`
	}
	if err := json.Unmarshal([]byte(projection.Messages[2].Content), &page); err != nil || page.Next <= 0 || !page.More {
		t.Fatal("invalid recovery cursor", err)
	}
}

func TestInstalledSkillHostToolIsAvailableWithoutApproval(t *testing.T) {
	id, err := domain.NewHostToolIdentity("skill")
	if err != nil {
		t.Fatal("installed skill reader is not a host tool:", err)
	}
	if !automaticallyApproves(nil, id) {
		t.Fatal("reading an installed skill requires approval")
	}
	for _, tool := range HostTools() {
		if tool.Definition.Name == "axlr_skill" && tool.Identity == id {
			return
		}
	}
	t.Fatal("model cannot call installed skill reader")
}

type skillPortFunc func(context.Context, string, string, string, int, int) (SkillPage, error)

func (f skillPortFunc) ReadSkill(ctx context.Context, plugin, skill, path string, offset, limit int) (SkillPage, error) {
	return f(ctx, plugin, skill, path, offset, limit)
}

func TestHostSkillReadsExactInstalledSkillByName(t *testing.T) {
	reader := skillPortFunc(func(_ context.Context, plugin, skill, path string, offset, limit int) (SkillPage, error) {
		if plugin != "visualization" || skill != "chart" || path != "SKILL.md" || offset != 0 || limit != 4096 {
			t.Fatalf("wrong request: %s %s %s %d %d", plugin, skill, path, offset, limit)
		}
		return SkillPage{Plugin: plugin, Skill: skill, Text: "# Chart guidance", TotalBytes: 16, NextOffsetBytes: 16}, nil
	})
	id, err := domain.NewHostToolIdentity("skill")
	if err != nil {
		t.Fatal(err)
	}
	out, err := (HostToolUseCase{Skills: reader}).Execute(context.Background(), domain.Session{}, id, hostJSON(t, `{"plugin":"visualization","skill":"chart"}`))
	if err != nil || out.IsError || !strings.Contains(string(out.Content), "# Chart guidance") {
		t.Fatalf("skill read failed: %+v %v", out, err)
	}
}

func TestHostSkillRejectsInvalidSelectorsBeforeReading(t *testing.T) {
	reader := skillPortFunc(func(context.Context, string, string, string, int, int) (SkillPage, error) {
		t.Fatal("invalid selector reached reader")
		return SkillPage{}, nil
	})
	id, err := domain.NewHostToolIdentity("skill")
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range []string{`{}`, `{"plugin":"../escape","skill":"chart"}`, `{"plugin":"visualization","skill":"../escape"}`, `{"plugin":"visualization","skill":"chart","offset_bytes":-1}`, `{"plugin":"visualization","skill":"chart","limit_bytes":5000}`, `{"plugin":"visualization","skill":"chart","extra":true}`} {
		out, err := (HostToolUseCase{Skills: reader}).Execute(context.Background(), domain.Session{}, id, hostJSON(t, args))
		if err != nil || !out.IsError {
			t.Fatalf("accepted %s: %+v %v", args, out, err)
		}
	}
}
