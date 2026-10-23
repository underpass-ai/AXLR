package domain

import (
	"strings"
	"testing"

	axlr "github.com/underpass-ai/AXLR/domain"
)

func policyTool(t *testing.T, name, kind, operation string) AvailableTool {
	t.Helper()
	parameters, _ := axlr.NewJSONObject([]byte(`{"type":"object"}`))
	tool := AvailableTool{Definition: axlr.ToolDefinition{Name: axlr.ToolName(name), Description: "tool", Parameters: parameters}}
	switch kind {
	case ToolKindLocal:
		tool.Identity = ToolIdentity{Kind: ToolKindLocal, LocalOperation: operation}
	case ToolKindHost:
		tool.Identity = ToolIdentity{Kind: ToolKindHost, LocalOperation: operation}
	default:
		plugin, native, _ := strings.Cut(operation, "/")
		tool.Identity = ToolIdentity{Kind: ToolKindPlugin, Plugin: axlr.PluginRef{PluginID: axlr.PluginID(plugin), ToolName: axlr.PluginToolName(native)}}
	}
	return tool
}

func policyArgs(t *testing.T, raw string) axlr.JSONValue {
	t.Helper()
	value, err := axlr.NewJSONObject([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func mustPolicy(t *testing.T, level PolicyLevel, allow, deny []PolicyRule) ExecutionPolicy {
	t.Helper()
	policy, err := NewExecutionPolicy(level, allow, deny)
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

// Every level, judged on a read, a local write, a command and an MCP tool,
// with and without an allow list that names them.
func TestExecutionPolicyLevels(t *testing.T) {
	read := policyTool(t, "local_read", ToolKindLocal, "read")
	write := policyTool(t, "local_write", ToolKindLocal, "write")
	command := policyTool(t, "local_exec", ToolKindLocal, "exec")
	query := policyTool(t, "mcp_0a1b", ToolKindPlugin, "observability/query_logs")
	plan := policyArgs(t, `{"path":"docs/plan.md","content":"x"}`)
	code := policyArgs(t, `{"path":"main.go","content":"x"}`)
	allowAll := []PolicyRule{{Name: "local_*"}, {Name: "observability/*"}}
	for name, tc := range map[string]struct {
		level    PolicyLevel
		allow    []PolicyRule
		tool     AvailableTool
		args     axlr.JSONValue
		verdict  PolicyVerdict
		contains string
	}{
		"observe reads":                       {PolicyObserve, nil, read, plan, PolicyAllow, ""},
		"observe refuses a write":             {PolicyObserve, nil, write, plan, PolicyForbid, "observes: local_write is not a read"},
		"observe refuses a command":           {PolicyObserve, allowAll, command, plan, PolicyForbid, "not a read"},
		"observe refuses an unlisted mcp":     {PolicyObserve, nil, query, plan, PolicyForbid, "not a read"},
		"observe runs a listed mcp":           {PolicyObserve, allowAll, query, plan, PolicyAllow, ""},
		"propose writes a plan":               {PolicyPropose, nil, write, plan, PolicyAllow, ""},
		"propose refuses code":                {PolicyPropose, allowAll, write, code, PolicyForbid, "proposes: local_write may only write a plan or a diff"},
		"propose writes a diff":               {PolicyPropose, nil, write, policyArgs(t, `{"path":"fix.patch","content":"x"}`), PolicyAllow, ""},
		"propose refuses a command":           {PolicyPropose, allowAll, command, plan, PolicyForbid, "proposes"},
		"propose runs a listed mcp":           {PolicyPropose, allowAll, query, plan, PolicyAllow, ""},
		"act-with-approval asks for a write":  {PolicyActWithApproval, allowAll, write, code, PolicyAsk, ""},
		"act-with-approval asks for an mcp":   {PolicyActWithApproval, nil, query, plan, PolicyAsk, ""},
		"act-with-approval reads":             {PolicyActWithApproval, nil, read, plan, PolicyAllow, ""},
		"act-within-policy runs a write":      {PolicyActWithinPolicy, allowAll, write, code, PolicyAllow, ""},
		"act-within-policy runs a command":    {PolicyActWithinPolicy, nil, command, plan, PolicyAllow, ""},
		"act-within-policy runs an mcp":       {PolicyActWithinPolicy, nil, query, plan, PolicyAllow, ""},
		"allow list forbids what it omits":    {PolicyActWithinPolicy, []PolicyRule{{Name: "local_read"}}, command, plan, PolicyForbid, "local_exec is not in the execution's allow list (local_read)"},
		"allow list keeps the read it names":  {PolicyObserve, []PolicyRule{{Name: "local_read"}}, read, plan, PolicyAllow, ""},
		"allow list forbids an unnamed read":  {PolicyObserve, []PolicyRule{{Name: "observability/*"}}, read, plan, PolicyForbid, "not in the execution's allow list"},
		"mcp matched by plugin and tool name": {PolicyObserve, []PolicyRule{{Name: "observability/query_logs"}}, query, plan, PolicyAllow, ""},
		"mcp matched by its own name":         {PolicyObserve, []PolicyRule{{Name: "query_logs"}}, query, plan, PolicyAllow, ""},
		"mcp matched by its alias":            {PolicyObserve, []PolicyRule{{Name: "mcp_0a1b"}}, query, plan, PolicyAllow, ""},
	} {
		t.Run(name, func(t *testing.T) {
			policy := mustPolicy(t, tc.level, tc.allow, nil)
			verdict, reason := policy.Judge(tc.tool, tc.args)
			if verdict != tc.verdict || !strings.Contains(reason, tc.contains) || (tc.verdict != PolicyForbid) != (reason == "") {
				t.Fatalf("verdict %d reason %q", verdict, reason)
			}
		})
	}
}

// A deny rule wins over an allow rule, by name or by arguments; an allow
// rule with argument patterns admits only the calls that satisfy them.
func TestExecutionPolicyRules(t *testing.T) {
	read := policyTool(t, "local_read", ToolKindLocal, "read")
	command := policyTool(t, "local_exec", ToolKindLocal, "exec")
	logs := policyArgs(t, `{"path":"logs/app/today.log"}`)
	secret := policyArgs(t, `{"path":"secrets/key.pem"}`)
	for name, tc := range map[string]struct {
		allow, deny []PolicyRule
		tool        AvailableTool
		args        axlr.JSONValue
		verdict     PolicyVerdict
		contains    string
	}{
		"deny by name wins over allow":        {[]PolicyRule{{Name: "local_*"}}, []PolicyRule{{Name: "local_exec"}}, command, logs, PolicyForbid, "local_exec is denied by the execution's policy (rule local_exec)"},
		"deny by arguments wins":              {[]PolicyRule{{Name: "local_read"}}, []PolicyRule{{Name: "local_read", Arguments: map[string]string{"path": "secrets/**"}}}, read, secret, PolicyForbid, "denied"},
		"deny by arguments spares the rest":   {[]PolicyRule{{Name: "local_read"}}, []PolicyRule{{Name: "local_read", Arguments: map[string]string{"path": "secrets/**"}}}, read, logs, PolicyAllow, ""},
		"allow pattern admits a match":        {[]PolicyRule{{Name: "local_read", Arguments: map[string]string{"path": "logs/**"}}}, nil, read, logs, PolicyAllow, ""},
		"allow pattern refuses a mismatch":    {[]PolicyRule{{Name: "local_read", Arguments: map[string]string{"path": "logs/**"}}}, nil, read, secret, PolicyForbid, "the arguments of local_read match no allow rule"},
		"allow pattern needs the argument":    {[]PolicyRule{{Name: "local_read", Arguments: map[string]string{"path": "logs/**"}}}, nil, read, policyArgs(t, `{"why":"x"}`), PolicyForbid, "match no allow rule"},
		"command program pattern":             {[]PolicyRule{{Name: "local_exec", Arguments: map[string]string{"program": "kubectl", "args": "*get*"}}}, nil, command, policyArgs(t, `{"program":"kubectl","args":["get","pods"]}`), PolicyAllow, ""},
		"command program pattern on a delete": {[]PolicyRule{{Name: "local_exec", Arguments: map[string]string{"program": "kubectl", "args": "*get*"}}}, nil, command, policyArgs(t, `{"program":"kubectl","args":["delete","pod"]}`), PolicyForbid, "match no allow rule"},
		"no lists: the level alone":           {nil, nil, command, logs, PolicyAllow, ""},
	} {
		t.Run(name, func(t *testing.T) {
			policy := mustPolicy(t, PolicyActWithinPolicy, tc.allow, tc.deny)
			verdict, reason := policy.Judge(tc.tool, tc.args)
			if verdict != tc.verdict || !strings.Contains(reason, tc.contains) {
				t.Fatalf("verdict %d reason %q", verdict, reason)
			}
		})
	}
}

// The model is offered only what the policy may allow: the bridge to MCP
// tools stays while one of them is offered, and the listing always.
func TestExecutionPolicyOffersTools(t *testing.T) {
	snapshot := []AvailableTool{
		policyTool(t, "local_read", ToolKindLocal, "read"),
		policyTool(t, "local_write", ToolKindLocal, "write"),
		policyTool(t, "local_exec", ToolKindLocal, "exec"),
		policyTool(t, "axlr_tools", ToolKindHost, HostOperationTools),
		policyTool(t, "axlr_call_tool", ToolKindHost, HostOperationCallTool),
		policyTool(t, "axlr_forge_tool", ToolKindHost, HostOperationForgeTool),
		policyTool(t, "query_logs", ToolKindPlugin, "observability/query_logs"),
		policyTool(t, "kmp_write_memory", ToolKindPlugin, "kmp/kmp_write_memory"),
	}
	names := func(tools []AvailableTool) string {
		out := make([]string, 0, len(tools))
		for _, tool := range tools {
			out = append(out, string(tool.Definition.Name))
		}
		return strings.Join(out, " ")
	}
	for name, tc := range map[string]struct {
		level       PolicyLevel
		allow, deny []PolicyRule
		want        string
	}{
		"observe without lists":           {PolicyObserve, nil, nil, "local_read axlr_tools"},
		"observe with the sre allow list": {PolicyObserve, []PolicyRule{{Name: "observability/*"}}, nil, "axlr_tools axlr_call_tool query_logs"},
		"propose":                         {PolicyPropose, nil, nil, "local_read local_write axlr_tools"},
		"act-with-approval":               {PolicyActWithApproval, nil, nil, "local_read local_write local_exec axlr_tools axlr_call_tool axlr_forge_tool query_logs kmp_write_memory"},
		"act-within-policy denying one":   {PolicyActWithinPolicy, nil, []PolicyRule{{Name: "kmp/*"}, {Name: "local_exec", Arguments: map[string]string{"program": "rm"}}}, "local_read local_write local_exec axlr_tools axlr_call_tool axlr_forge_tool query_logs"},
		"allow list of reads":             {PolicyActWithinPolicy, []PolicyRule{{Name: "local_read"}, {Name: "local_search"}}, nil, "local_read axlr_tools"},
		"bridge denied by name":           {PolicyActWithinPolicy, nil, []PolicyRule{{Name: "axlr_call_tool"}, {Name: "axlr_tools"}}, "local_read local_write local_exec axlr_forge_tool query_logs kmp_write_memory"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := names(mustPolicy(t, tc.level, tc.allow, tc.deny).OfferedTools(snapshot)); got != tc.want {
				t.Fatalf("offered %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExecutionPolicyValidation(t *testing.T) {
	for name, tc := range map[string]struct {
		level PolicyLevel
		rule  PolicyRule
	}{
		"unknown level":        {"autonomous", PolicyRule{Name: "local_read"}},
		"empty name":           {PolicyObserve, PolicyRule{}},
		"name with a space":    {PolicyObserve, PolicyRule{Name: "local read"}},
		"empty argument key":   {PolicyObserve, PolicyRule{Name: "local_read", Arguments: map[string]string{"": "x"}}},
		"empty pattern":        {PolicyObserve, PolicyRule{Name: "local_read", Arguments: map[string]string{"path": ""}}},
		"control in a pattern": {PolicyObserve, PolicyRule{Name: "local_read", Arguments: map[string]string{"path": "a\x00b"}}},
		"long name":            {PolicyObserve, PolicyRule{Name: strings.Repeat("a", 129)}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewExecutionPolicy(tc.level, []PolicyRule{tc.rule}, nil); err == nil {
				t.Fatal("accepted")
			}
		})
	}
	policy := mustPolicy(t, PolicyObserve, []PolicyRule{{Name: "local_read", Arguments: map[string]string{"path": "logs/**"}}}, nil)
	rules := policy.Allow()
	rules[0].Arguments["path"] = "changed"
	if policy.Allow()[0].Arguments["path"] != "logs/**" || policy.Level() != PolicyObserve || len(policy.Deny()) != 0 || !policy.AsksForEachWrite() == (policy.Level() == PolicyActWithApproval) {
		t.Fatalf("%+v", policy)
	}
}

func TestPolicyGlob(t *testing.T) {
	for _, tc := range []struct {
		pattern, value string
		match          bool
	}{
		{"logs/**", "logs/app/today.log", true},
		{"logs/**", "logs", false},
		{"logs/*", "logs/today.log", true},
		{"logs/*", "logs/app/today.log", false},
		{"**/*.log", "logs/app/today.log", true},
		{"**/*.log", "today.log", true},
		{"a/**/b", "a/b", true},
		{"a/**/b", "a/x/y/b", true},
		{"kubectl", "kubectl", true},
		{"kubectl", "kubectl-proxy", false},
		{"local_?ead", "local_read", true},
		{"local_?ead", "local_/ead", false},
		{"observability/*", "observability/query_logs", true},
		{"*", "", true},
		{"", "", true},
		{"", "x", false},
	} {
		if got := globMatch(tc.pattern, tc.value); got != tc.match {
			t.Errorf("%q on %q: %v", tc.pattern, tc.value, got)
		}
	}
}

// A call the policy refused is "forbidden" in the result, apart from a
// call the person or the model's deny reason denied.
func TestExecutionResultReportsForbiddenCalls(t *testing.T) {
	s, err := NewSession("0123456789abcdef0123456789abcdef", "/tmp/work", "test/model")
	if err != nil {
		t.Fatal(err)
	}
	tools := []AvailableTool{policyTool(t, "local_exec", ToolKindLocal, "exec")}
	if err := s.BeginTurn("run it", tools); err != nil {
		t.Fatal(err)
	}
	args := policyArgs(t, `{"program":"touch","args":["x"]}`)
	calls := []axlr.ToolCall{{ID: "exec-1", Name: "local_exec", Arguments: args}, {ID: "exec-2", Name: "local_exec", Arguments: args}}
	if err := s.CompleteAssistant(axlr.CompletionResult{Message: axlr.Message{Role: axlr.RoleAssistant, ToolCalls: calls}}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordToolOutcome("exec-1", DecisionDeny, ToolOutcome{Content: ForbiddenOutcome("local_exec is not a read"), IsError: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordToolOutcome("exec-2", DecisionDeny, ToolOutcome{Content: "tool call denied by user", IsError: true}); err != nil {
		t.Fatal(err)
	}
	result := NewExecutionResult(s, nil)
	if len(result.Calls) != 2 || result.Calls[0].Status != ExecutionCallForbidden || result.Calls[1].Status != ExecutionCallDenied {
		t.Fatalf("%+v", result.Calls)
	}
}

// The policy survives an export and a restore, and a restore refuses an
// invalid one.
func TestSessionKeepsItsPolicy(t *testing.T) {
	s, err := NewSession("0123456789abcdef0123456789abcdef", "/tmp/work", "test/model")
	if err != nil {
		t.Fatal(err)
	}
	if _, has := s.Policy(); has {
		t.Fatal("a new session has a policy")
	}
	policy := mustPolicy(t, PolicyObserve, []PolicyRule{{Name: "local_read", Arguments: map[string]string{"path": "logs/**"}}}, []PolicyRule{{Name: "local_exec"}})
	if err := s.SetPolicy(policy); err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreSession(s.Export())
	if err != nil {
		t.Fatal(err)
	}
	kept, has := restored.Policy()
	if !has || kept.Level() != PolicyObserve || len(kept.Allow()) != 1 || kept.Allow()[0].Arguments["path"] != "logs/**" || len(kept.Deny()) != 1 {
		t.Fatalf("%+v", kept)
	}
	state := s.Export()
	state.Policy = &ExecutionPolicy{level: "bogus"}
	if _, err := RestoreSession(state); err == nil {
		t.Fatal("restored an invalid policy")
	}
	s.ClearPolicy()
	if _, has := s.Policy(); has {
		t.Fatal("the policy was not cleared")
	}
}
