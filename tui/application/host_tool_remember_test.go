package application

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type rememberRecorder struct {
	requests []RememberRequest
	answer   map[string]any
}

func (r *rememberRecorder) Remember(_ context.Context, request RememberRequest) (map[string]any, error) {
	r.requests = append(r.requests, request)
	return r.answer, nil
}

func rememberIdentity(t *testing.T) domain.ToolIdentity {
	t.Helper()
	id, err := domain.NewHostToolIdentity(domain.HostOperationRemember)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// axlr_remember writes KMP memory, so it is refused, and approved, exactly
// where kmp_write_memory is.
func TestAxlrRememberIsJudgedAndApprovedAsKMPWriteMemory(t *testing.T) {
	remember := rememberIdentity(t)
	for _, mode := range []domain.WorkMode{domain.ModeNormal, domain.ModeReview, domain.ModeWriter, domain.ModeResearch, domain.ModeDebug, domain.ModeIncident, domain.ModeRepair, domain.ModeImprove, domain.ModePlan, domain.ModeTask} {
		got, _ := mode.Judge(remember, root.JSONValue{})
		want, _ := mode.Judge(domain.MemoryWriteIdentity, root.JSONValue{})
		if got != want {
			t.Fatalf("%s: axlr_remember %v, kmp_write_memory %v", mode, got, want)
		}
	}
	if verdict, _ := domain.ModeRepair.Judge(remember, root.JSONValue{}); verdict != domain.VerdictDeny {
		t.Fatal("repair mode let the model write memory")
	}
	approvesWrite := approvalFunc(func(id domain.ToolIdentity) bool { return id == domain.MemoryWriteIdentity })
	if !automaticallyApproves(approvesWrite, remember) || automaticallyApproves(approvalFunc(func(domain.ToolIdentity) bool { return false }), remember) || automaticallyApproves(nil, remember) {
		t.Fatal("axlr_remember is not approved as kmp_write_memory")
	}
}

func rememberSession(t *testing.T, mode domain.WorkMode) domain.Session {
	t.Helper()
	s := turnSession(t)
	if mode != domain.ModeNormal {
		if err := s.SetMode(mode); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.BeginTurn("record what we decided", memoryTools(t)); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAxlrRememberIsOfferedWhereTheModelRecordsMemory(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mode    domain.WorkMode
		port    bool
		offered bool
	}{
		{"normal with KMP", domain.ModeNormal, true, true},
		{"without the port", domain.ModeNormal, false, false},
		{"repair refuses memory writes", domain.ModeRepair, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := rememberSession(t, tc.mode)
			var request root.CompletionRequest
			u := ContinueTurnUseCase{Store: &memoryStore{}, Models: streamFunc(func(_ context.Context, req root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
				request = req
				return assistant("ok"), nil
			})}
			if tc.port {
				u.Remember = &rememberRecorder{}
			}
			if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil {
				t.Fatal(err)
			}
			if requestHasTool(request, HostRememberName) != tc.offered {
				t.Fatalf("offered = %v", !tc.offered)
			}
			guidance := string(request.Messages[0].Content)
			if tc.offered != strings.Contains(guidance, "write it with axlr_remember") {
				t.Fatalf("guidance: %s", guidance)
			}
		})
	}
}

func TestHostRememberBuildsTheRequestFromTheSession(t *testing.T) {
	s := rememberSession(t, domain.ModeNormal)
	labels := &contextLabels{labels: map[domain.SessionID]domain.SessionLabel{s.Export().ID: {About: "project:AXLR"}}}
	recorder := &rememberRecorder{answer: map[string]any{"accepted": true}}
	u := HostToolUseCase{Memory: recorder, Labels: labels}
	run := func(arguments string) (domain.ToolOutcome, error) {
		return u.Execute(context.Background(), s, rememberIdentity(t), mustObject(t, arguments))
	}
	if outcome, err := run(`{"kind":"decision","text":"Plan with the session model.","evidence":["plan.model default"],"links":[{"ref":"project:AXLR:entry:x","rel":"supports","why":"same release"}]}`); err != nil || outcome.IsError {
		t.Fatalf("%+v %v", outcome, err)
	}
	if _, err := run(`{"kind":"decision","text":"Plan with the session model.","evidence":["again"]}`); err != nil {
		t.Fatal(err)
	}
	if _, err := run(`{"kind":"decision","text":"Something else.","evidence":["e"],"about":"project:other"}`); err != nil {
		t.Fatal(err)
	}
	first, retry, other := recorder.requests[0], recorder.requests[1], recorder.requests[2]
	if first.About != "project:AXLR" || first.Kind != "decision" || len(first.Links) != 1 || first.Links[0].Rel != "supports" || first.Labels["session"][0] != string(s.Export().ID) || first.Labels["ws"][0] != string(s.Export().Workspace) {
		t.Fatalf("request %+v", first)
	}
	// The key follows what is recorded, so a retry never duplicates it.
	if !strings.HasPrefix(first.IdempotencyKey, "axlr:remember:") || retry.IdempotencyKey != first.IdempotencyKey || other.IdempotencyKey == first.IdempotencyKey || other.About != "project:other" {
		t.Fatalf("keys %q %q %q", first.IdempotencyKey, retry.IdempotencyKey, other.IdempotencyKey)
	}
	if _, err := run(`{"continuation":"read_1"}`); err != nil || recorder.requests[3].Continuation != "read_1" || recorder.requests[3].About != "" {
		t.Fatalf("continuation %+v %v", recorder.requests[3], err)
	}
	for _, refused := range []string{`{"continuation":"read_1","text":"x"}`, `{"kind":"decision","text":"no evidence"}`} {
		if outcome, _ := run(refused); !outcome.IsError {
			t.Fatalf("accepted %s", refused)
		}
	}
	// Without KMP, or while a ceremony records the outcome, it refuses.
	if outcome, _ := (HostToolUseCase{}).Execute(context.Background(), s, rememberIdentity(t), mustObject(t, `{"kind":"decision","text":"x","evidence":["e"]}`)); !outcome.IsError || !strings.Contains(string(outcome.Content), "KMP is not connected") {
		t.Fatalf("without KMP: %s", outcome.Content)
	}
	if err := s.SetCeremony(domain.CeremonyRun{Definition: "axlr_debug", Version: "2.0", Instance: "i", Step: "repair", Iteration: 1}); err != nil {
		t.Fatal(err)
	}
	if outcome, _ := run(`{"kind":"decision","text":"x","evidence":["e"]}`); !outcome.IsError || !strings.Contains(string(outcome.Content), "leaves it to the console") {
		t.Fatalf("in a ceremony: %s", outcome.Content)
	}
}

func TestAxlrRememberCountsAsTheRequestsMemoryWrite(t *testing.T) {
	edit := toolCall(t, "e1", "local_edit", `{"path":"a.go","old_text":"x","new_text":"y"}`)
	if needsMemoryReminder(memoryRequest(t, edit, toolCall(t, "m1", HostRememberName, `{"kind":"decision","text":"x","evidence":["e"]}`)), "kmp_write_memory") {
		t.Fatal("a request that called axlr_remember was reminded")
	}
	// The checkpoint lists what it recorded, from its answer.
	messages := []root.Message{
		{Role: root.RoleUser, Content: "decide"},
		{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{toolCall(t, "m1", HostRememberName, `{"kind":"decision","text":"x","evidence":["e"]}`), toolCall(t, "m2", HostRememberName, `{"kind":"decision","text":"y","evidence":["e"]}`)}},
		{Role: root.RoleTool, ToolCallID: "m1", Content: `{"accepted":true,"about":"project:AXLR","idempotency_key":"axlr:remember:aa","ref":"project:AXLR:entry:x"}`},
		{Role: root.RoleTool, ToolCallID: "m2", Content: `{"accepted":false,"needs_review":true,"about":"project:AXLR","continuation":"read_1"}`},
		{Role: root.RoleAssistant, Content: "done"},
	}
	records := omittedTurnRecords(messages, 0, len(messages))
	encoded, _ := json.Marshal(records[0].memory)
	if string(encoded) != `[{"about":"project:AXLR","idempotency_key":"axlr:remember:aa"}]` {
		t.Fatalf("memory written %s", encoded)
	}
}

// A closed turn's exact schema is left out of the request: the model read it
// to make one call, which is done.
func TestClosedTurnsLeaveExactSchemasOut(t *testing.T) {
	schema := `{"name":"kmp_write_memory","input_schema":` + strings.Repeat(`{"type":"object"},`, 400) + `{}}`
	read := toolCall(t, "s1", "axlr_tools", `{"name":"kmp_write_memory"}`)
	search := toolCall(t, "s2", "axlr_tools", `{"query":"memory"}`)
	current := toolCall(t, "s3", "axlr_tools", `{"name":"kmp_wake"}`)
	listing := `{"tools":[` + strings.Repeat(`{"name":"kmp_x","summary":"y"},`, 40) + `{}]}`
	original := []root.Message{
		{Role: root.RoleUser, Content: "recuerda"},
		{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{read, search}},
		{Role: root.RoleTool, ToolCallID: "s1", Content: root.Text(schema)},
		{Role: root.RoleTool, ToolCallID: "s2", Content: root.Text(listing)},
		{Role: root.RoleAssistant, Content: "hecho"},
		{Role: root.RoleUser, Content: "otra"},
		{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{current}},
		{Role: root.RoleTool, ToolCallID: "s3", Content: root.Text(schema)},
	}
	projection, err := NewDefaultModelContextProjector().Project(original)
	if err != nil {
		t.Fatal(err)
	}
	var stub map[string]any
	if err := json.Unmarshal([]byte(projection.Messages[2].Content), &stub); err != nil || stub["name"] != "kmp_write_memory" || stub["schema_omitted_bytes"] != float64(len(schema)) || !strings.Contains(stub["recover"].(string), "axlr_tools") {
		t.Fatalf("closed schema: %s (%v)", projection.Messages[2].Content, err)
	}
	if string(projection.Messages[3].Content) != listing || string(projection.Messages[7].Content) != schema || string(original[2].Content) != schema {
		t.Fatal("a listing, the open turn's schema or the saved transcript changed")
	}
}
