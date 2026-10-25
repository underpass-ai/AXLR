package domain

import (
	"encoding/json"
	"strings"
	"testing"

	axlr "github.com/underpass-ai/AXLR/domain"
)

// axlr_deliver's arguments: exactly one of path and content, a safe name,
// a bare media type and one line of description, within the bounds.
func TestParseDeliverable(t *testing.T) {
	big, _ := json.Marshal(strings.Repeat("x", MaxDeliverableInlineBytes+1))
	for name, tc := range map[string]struct {
		raw      string
		inline   bool
		path     string
		contains string
	}{
		"a file by path":          {raw: `{"why":"done","name":"report.md","media_type":"text/markdown","description":"The report","path":"./out//report.md"}`, path: "out/report.md"},
		"inline content":          {raw: `{"name":"plan.md","media_type":"text/markdown","description":"The plan","content":"# Plan"}`, inline: true},
		"both":                    {raw: `{"name":"a.md","media_type":"text/markdown","description":"d","path":"a.md","content":"x"}`, contains: "exactly one of path"},
		"neither":                 {raw: `{"name":"a.md","media_type":"text/markdown","description":"d"}`, contains: "exactly one of path"},
		"parent traversal":        {raw: `{"name":"a.md","media_type":"text/markdown","description":"d","path":"../secret"}`, contains: "path:"},
		"absolute path":           {raw: `{"name":"a.md","media_type":"text/markdown","description":"d","path":"/etc/passwd"}`, contains: "path:"},
		"a name with a separator": {raw: `{"name":"dir/a.md","media_type":"text/markdown","description":"d","content":"x"}`, contains: "name must be"},
		"a dotted name":           {raw: `{"name":"..md","media_type":"text/markdown","description":"d","content":"x"}`, contains: "name must be"},
		"a hidden name":           {raw: `{"name":".env","media_type":"text/plain","description":"d","content":"x"}`, contains: "name must be"},
		"a device name":           {raw: `{"name":"NUL.txt","media_type":"text/plain","description":"d","content":"x"}`, contains: "device name"},
		"a media type parameter":  {raw: `{"name":"a.txt","media_type":"text/plain; charset=utf-8","description":"d","content":"x"}`, contains: "media_type"},
		"an upper-case type":      {raw: `{"name":"a.txt","media_type":"Text/Plain","description":"d","content":"x"}`, contains: "media_type"},
		"two lines":               {raw: `{"name":"a.txt","media_type":"text/plain","description":"one\ntwo","content":"x"}`, contains: "one line"},
		"no description":          {raw: `{"name":"a.txt","media_type":"text/plain","description":" ","content":"x"}`, contains: "one line"},
		"empty content":           {raw: `{"name":"a.txt","media_type":"text/plain","description":"d","content":""}`, contains: "content is empty"},
		"content past the bound":  {raw: `{"name":"a.txt","media_type":"text/plain","description":"d","content":` + string(big) + `}`, contains: "exceeds"},
		"an unknown argument":     {raw: `{"name":"a.txt","media_type":"text/plain","description":"d","content":"x","mode":"0777"}`, contains: "takes name"},
	} {
		t.Run(name, func(t *testing.T) {
			d, err := ParseDeliverable(policyArgs(t, tc.raw))
			if tc.contains != "" {
				if err == nil || !strings.Contains(err.Error(), tc.contains) {
					t.Fatalf("err %v", err)
				}
				return
			}
			if err != nil || d.Inline() != tc.inline || d.Path != tc.path {
				t.Fatalf("%+v %v", d, err)
			}
		})
	}
	d, _ := ParseDeliverable(policyArgs(t, `{"name":"plan.md","media_type":"text/markdown","description":"The plan","content":"# Plan"}`))
	if got := d.Target("0123456789abcdef0123456789abcdef"); got != ".axlr/deliverables/0123456789abcdef0123456789abcdef/plan.md" {
		t.Fatal(got)
	}
	if !DeliversInline(policyArgs(t, `{"content":"x","path":"a"}`)) || DeliversInline(policyArgs(t, `{"path":"a"}`)) {
		t.Fatal("content makes a delivery a write")
	}
}

// The levels judge a delivery by path as a read and inline content as a
// write of the deliverables file; an allow list must name axlr_deliver.
func TestExecutionPolicyJudgesDeliveries(t *testing.T) {
	deliver := policyTool(t, "axlr_deliver", ToolKindHost, HostOperationDeliver)
	byPath := policyArgs(t, `{"name":"r.json","media_type":"application/json","description":"d","path":"out/r.json"}`)
	plan := policyArgs(t, `{"name":"plan.md","media_type":"text/markdown","description":"d","content":"x"}`)
	data := policyArgs(t, `{"name":"r.json","media_type":"application/json","description":"d","content":"{}"}`)
	for name, tc := range map[string]struct {
		level    PolicyLevel
		allow    []PolicyRule
		args     axlr.JSONValue
		verdict  PolicyVerdict
		contains string
	}{
		"observe delivers a file":                {PolicyObserve, nil, byPath, PolicyAllow, ""},
		"observe refuses content":                {PolicyObserve, nil, plan, PolicyForbid, "observes: axlr_deliver may deliver a workspace file by path"},
		"propose writes a plan":                  {PolicyPropose, nil, plan, PolicyAllow, ""},
		"propose refuses data inline":            {PolicyPropose, nil, data, PolicyForbid, "proposes: axlr_deliver may write only a plan or a diff"},
		"propose delivers data by path":          {PolicyPropose, nil, byPath, PolicyAllow, ""},
		"act-with-approval asks for content":     {PolicyActWithApproval, nil, data, PolicyAsk, ""},
		"act-with-approval delivers a file":      {PolicyActWithApproval, nil, byPath, PolicyAllow, ""},
		"act-within-policy writes content":       {PolicyActWithinPolicy, nil, data, PolicyAllow, ""},
		"an allow list without it forbids it":    {PolicyObserve, []PolicyRule{{Name: "local_read"}}, byPath, PolicyForbid, "not in the execution's allow list"},
		"an allow list matches its path":         {PolicyObserve, []PolicyRule{{Name: "axlr_deliver", Arguments: map[string]string{"path": "out/**"}}}, byPath, PolicyAllow, ""},
		"an allow list refuses another path":     {PolicyObserve, []PolicyRule{{Name: "axlr_deliver", Arguments: map[string]string{"path": "reports/**"}}}, byPath, PolicyForbid, "match no allow rule"},
		"an allow list still observes the level": {PolicyObserve, []PolicyRule{{Name: "axlr_deliver"}}, plan, PolicyForbid, "observes"},
	} {
		t.Run(name, func(t *testing.T) {
			policy := mustPolicy(t, tc.level, tc.allow, nil)
			verdict, reason := policy.Judge(deliver, tc.args)
			if verdict != tc.verdict || !strings.Contains(reason, tc.contains) || (tc.verdict != PolicyForbid) != (reason == "") {
				t.Fatalf("verdict %d reason %q", verdict, reason)
			}
		})
	}
	// It is offered at every level, since a file by path is a read.
	for _, level := range []PolicyLevel{PolicyObserve, PolicyPropose, PolicyActWithApproval, PolicyActWithinPolicy} {
		if offered := mustPolicy(t, level, nil, nil).OfferedTools([]AvailableTool{deliver}); len(offered) != 1 {
			t.Fatalf("%s offers %v", level, offered)
		}
	}
	if offered := mustPolicy(t, PolicyActWithinPolicy, nil, []PolicyRule{{Name: "axlr_deliver"}}).OfferedTools([]AvailableTool{deliver}); len(offered) != 0 {
		t.Fatalf("a deny rule hides it: %v", offered)
	}
}

// The work modes refuse inline content where they refuse a write of its
// name; a file by path is a read every mode allows.
func TestWorkModesJudgeDeliveries(t *testing.T) {
	id := ToolIdentity{Kind: ToolKindHost, LocalOperation: HostOperationDeliver}
	byPath := policyArgs(t, `{"name":"r.json","media_type":"application/json","description":"d","path":"out/r.json"}`)
	plan := policyArgs(t, `{"name":"plan.md","media_type":"text/markdown","description":"d","content":"x"}`)
	data := policyArgs(t, `{"name":"r.json","media_type":"application/json","description":"d","content":"{}"}`)
	for name, tc := range map[string]struct {
		mode    WorkMode
		args    axlr.JSONValue
		verdict ModeVerdict
	}{
		"normal writes data":        {ModeNormal, data, VerdictAllow},
		"review delivers a file":    {ModeReview, byPath, VerdictAllow},
		"review refuses content":    {ModeReview, plan, VerdictDeny},
		"writer writes a document":  {ModeWriter, plan, VerdictAllow},
		"writer refuses data":       {ModeWriter, data, VerdictDeny},
		"research writes a doc":     {ModeResearch, plan, VerdictAllow},
		"research delivers a file":  {ModeResearch, byPath, VerdictAllow},
		"research refuses data":     {ModeResearch, data, VerdictDeny},
		"writer delivers data file": {ModeWriter, byPath, VerdictAllow},
	} {
		t.Run(name, func(t *testing.T) {
			if verdict, reason := tc.mode.Judge(id, tc.args); verdict != tc.verdict || (verdict == VerdictDeny) == (reason == "") {
				t.Fatalf("verdict %d reason %q", verdict, reason)
			}
		})
	}
}

// The result reads each artifact from its call's outcome, keeps the last
// delivery of a reference, and adds the predecessors' ahead of its own.
func TestExecutionResultListsArtifacts(t *testing.T) {
	deliverCatalog := func(t *testing.T) []AvailableTool {
		return append(catalog(t), AvailableTool{Definition: axlr.ToolDefinition{Name: DeliverToolName, Parameters: object(t)}, Identity: ToolIdentity{Kind: ToolKindHost, LocalOperation: HostOperationDeliver}})
	}
	call := func(id string) axlr.ToolCall {
		return axlr.ToolCall{ID: axlr.ToolCallID(id), Name: DeliverToolName, Arguments: object(t)}
	}
	outcome := func(name, reference, sum string, source ArtifactSource) ToolOutcome {
		encoded, _ := json.Marshal(DeliveredArtifact{Artifact: DeliveredArtifactFields{Name: name, MediaType: "text/markdown", Description: "d", Size: 4, SHA256: strings.Repeat(sum, 64), Reference: reference, Source: source}})
		return ToolOutcome{Content: axlr.Text(`{"call_id":"x",` + string(encoded[1:]))}
	}
	s := session(t)
	must(t, s.BeginTurn("deliver", deliverCatalog(t)))
	must(t, s.CompleteAssistant(axlr.CompletionResult{Message: axlr.Message{Role: axlr.RoleAssistant, ToolCalls: []axlr.ToolCall{call("d1"), call("d2"), call("d3"), call("d4"), call("d5")}}}))
	must(t, s.RecordToolOutcome("d1", DecisionAutoApprove, outcome("plan.md", ".axlr/deliverables/s/plan.md", "a", ArtifactInline)))
	must(t, s.RecordToolOutcome("d2", DecisionApprove, outcome("report.md", "out/report.md", "b", ArtifactFile)))
	must(t, s.RecordToolOutcome("d3", DecisionAutoApprove, outcome("plan.md", ".axlr/deliverables/s/plan.md", "c", ArtifactInline)))
	must(t, s.RecordToolOutcome("d4", DecisionDeny, ToolOutcome{Content: "tool call denied by user", IsError: true}))
	must(t, s.RecordToolOutcome("d5", DecisionAutoApprove, ToolOutcome{Content: `{"error":"out/x.md cannot be delivered"}`, IsError: true}))
	result := NewExecutionResult(s, nil)
	if len(result.Artifacts) != 2 || result.Artifacts[0].Reference != "out/report.md" || result.Artifacts[1].Call != "d3" || result.Artifacts[1].SHA256 != strings.Repeat("c", 64) || result.Artifacts[1].Session != s.ID() || result.Artifacts[0].Source != ArtifactFile {
		t.Fatalf("%+v", result.Artifacts)
	}
	predecessor, _ := NewSession("fedcba9876543210fedcba9876543210", s.Export().Workspace, "test/model")
	must(t, predecessor.BeginTurn("deliver", deliverCatalog(t)))
	must(t, predecessor.CompleteAssistant(axlr.CompletionResult{Message: axlr.Message{Role: axlr.RoleAssistant, ToolCalls: []axlr.ToolCall{call("p1"), call("p2")}}}))
	must(t, predecessor.RecordToolOutcome("p1", DecisionAutoApprove, outcome("notes.md", "notes.md", "d", ArtifactFile)))
	must(t, predecessor.RecordToolOutcome("p2", DecisionAutoApprove, outcome("report.md", "out/report.md", "e", ArtifactFile)))
	result.Delivered(predecessor)
	if len(result.Artifacts) != 3 || result.Artifacts[0].Reference != "notes.md" || result.Artifacts[0].Session != predecessor.ID() || result.Artifacts[1].Call != "d2" || result.Artifacts[2].Call != "d3" {
		t.Fatalf("%+v", result.Artifacts)
	}
}
