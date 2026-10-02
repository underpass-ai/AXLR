package domain

import (
	"testing"

	axlr "github.com/underpass-ai/AXLR/domain"
)

func jsonArgs(t *testing.T, raw string) axlr.JSONValue {
	t.Helper()
	value, err := axlr.NewJSONObject([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func localTool(t *testing.T, operation string) ToolIdentity {
	t.Helper()
	id, err := NewLocalToolIdentity(operation)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestParseWorkModeAcceptsOnlyKnownModes(t *testing.T) {
	for _, raw := range []string{"normal", "review", "writer", "research"} {
		if mode, err := ParseWorkMode(raw); err != nil || string(mode) != raw {
			t.Fatalf("%s: %v", raw, err)
		}
	}
	for _, raw := range []string{"", "Writer", "loud", "review "} {
		if _, err := ParseWorkMode(raw); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
}

func TestModesJudgeWorkspaceChanges(t *testing.T) {
	write, edit := localTool(t, "write"), localTool(t, "edit")
	exec, read := localTool(t, "exec"), localTool(t, "read")
	plugin, _ := NewPluginToolIdentity(axlr.PluginRef{PluginID: "kmp", ToolName: "kmp_write_memory"})
	doc := jsonArgs(t, `{"path":"docs/guide.go","content":"x"}`)
	readme := jsonArgs(t, `{"path":"README.md","content":"x"}`)
	code := jsonArgs(t, `{"path":"wc.py","content":"x"}`)
	sneaky := jsonArgs(t, `{"path":"docs/../wc.py","content":"x"}`)
	cases := []struct {
		mode WorkMode
		id   ToolIdentity
		args axlr.JSONValue
		want ModeVerdict
	}{
		{ModeNormal, write, code, VerdictAllow},
		{ModeNormal, exec, jsonArgs(t, `{}`), VerdictAllow},
		{ModeReview, write, readme, VerdictDeny},
		{ModeReview, edit, readme, VerdictDeny},
		{ModeReview, exec, jsonArgs(t, `{}`), VerdictAsk},
		{ModeReview, read, code, VerdictAllow},
		{ModeReview, plugin, jsonArgs(t, `{}`), VerdictAllow},
		{ModeWriter, write, readme, VerdictAllow},
		{ModeWriter, edit, doc, VerdictAllow},
		{ModeWriter, write, code, VerdictDeny},
		{ModeWriter, write, sneaky, VerdictDeny},
		{ModeWriter, exec, jsonArgs(t, `{}`), VerdictAsk},
		{ModeResearch, write, readme, VerdictAllow},
		{ModeResearch, edit, code, VerdictDeny},
		{ModeResearch, exec, jsonArgs(t, `{}`), VerdictAsk},
		{ModeWriter, exec, jsonArgs(t, `{"program":"python3","args":["-"],"stdin":"open('wc.py','w')"}`), VerdictDeny},
		{ModeReview, exec, jsonArgs(t, `{"program":"sh","stdin":"rm -rf x"}`), VerdictDeny},
		{ModeNormal, exec, jsonArgs(t, `{"program":"python3","stdin":"print(1)"}`), VerdictAllow},
	}
	for _, c := range cases {
		got, why := c.mode.Judge(c.id, c.args)
		if got != c.want {
			t.Fatalf("%s %s %s: got %d want %d (%s)", c.mode, c.id.LocalOperation, c.args.Bytes(), got, c.want, why)
		}
		if got == VerdictDeny && why == "" {
			t.Fatalf("%s denial has no reason", c.mode)
		}
	}
}

func TestOnlyReviewHidesWriteTools(t *testing.T) {
	if !ModeReview.HidesWriteTools() || ModeWriter.HidesWriteTools() || ModeNormal.HidesWriteTools() || ModeResearch.HidesWriteTools() {
		t.Fatal("unexpected write-tool visibility")
	}
}
