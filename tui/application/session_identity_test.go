package application

import (
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestWithSessionContextPutsTheSessionContextOnKMPCallsOnly(t *testing.T) {
	s := turnSession(t)
	if err := s.SetInstance(domain.SessionInstance{KMPAgent: "agent_1", KMPContext: "ctx_1"}); err != nil {
		t.Fatal(err)
	}
	kmp := func(name string) domain.AvailableTool {
		identity, _ := domain.NewPluginToolIdentity(root.PluginRef{PluginID: "kmp", ToolName: root.PluginToolName(name)})
		return domain.AvailableTool{Identity: identity}
	}
	args := func(text string) root.JSONValue {
		value, err := root.NewJSONObject([]byte(text))
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	cases := []struct {
		tool domain.AvailableTool
		in   string
		want string
	}{
		{kmp("kmp_ask"), `{"about":"project:AXLR","question":"q"}`, `{"about":"project:AXLR","context_id":"ctx_1","question":"q"}`},
		{kmp("kmp_write_memory"), `{"about":"x","context_id":"ctx_model"}`, `{"about":"x","context_id":"ctx_1"}`},
		{kmp("kmp_wake"), `{"about":"x","context_id":""}`, `{"about":"x","context_id":"ctx_1"}`},
		{kmp("kmp_guide"), `{"registration_key":"k"}`, `{"registration_key":"k"}`},
		{turnTools()[0], `{"program":"ls"}`, `{"program":"ls"}`},
	}
	for _, tc := range cases {
		got := withSessionContext(s, tc.tool, args(tc.in))
		if string(got.Compact().Bytes()) != tc.want {
			t.Fatalf("%s %s: %s", tc.tool.Identity.Plugin.ToolName, tc.in, got.Bytes())
		}
	}
	other := turnSession(t)
	if got := withSessionContext(other, kmp("kmp_ask"), args(`{"about":"x"}`)); string(got.Compact().Bytes()) != `{"about":"x"}` {
		t.Fatalf("without a context: %s", got.Bytes())
	}
}
