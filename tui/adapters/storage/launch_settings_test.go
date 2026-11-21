package storage

import (
	"slices"
	"testing"
)

func TestLaunchSettingsChangedNamesTheKeysReadOnlyAtLaunch(t *testing.T) {
	launch := UserSettings{ContextTokens: 0, PromptTokens: 75000, LocalModels: []LocalModel{{ID: "local/qwen", URL: "http://127.0.0.1:18083/v1", Model: "qwen", ContextTokens: 16384}}}
	same := launch
	same.LocalModels = append([]LocalModel(nil), launch.LocalModels...)
	if got := LaunchSettingsChanged(launch, same); len(got) != 0 {
		t.Fatalf("unchanged settings reported %v", got)
	}
	// The window raised in another editor (20 November 2026).
	raised := same
	raised.LocalModels = []LocalModel{launch.LocalModels[0]}
	raised.LocalModels[0].ContextTokens = 90000
	raised.PromptTokens = 64000
	raised.Sandbox = &ExecSandboxSettings{Mode: "process"}
	if got := LaunchSettingsChanged(launch, raised); !slices.Equal(got, []string{"local_models", "prompt_tokens", "exec_sandbox"}) {
		t.Fatalf("changed = %v", got)
	}
	// The console's own writes are no change to report.
	own := same
	own.Model = "local/other"
	if got := LaunchSettingsChanged(launch, own); len(got) != 0 {
		t.Fatalf("the console's own write reported %v", got)
	}
}

// An empty list written by hand is the absent one the console's own writes
// leave (omitempty): no restart notice at every prompt for it. Other keys
// read only at launch are reported too (21 November 2026).
func TestLaunchSettingsTreatAnEmptyListAsAbsentAndWatchEveryLaunchKey(t *testing.T) {
	launch := UserSettings{LocalModels: []LocalModel{}, Sandbox: &ExecSandboxSettings{Mode: "process", Writable: []string{}}}
	rewritten := UserSettings{Sandbox: &ExecSandboxSettings{Mode: "process"}}
	if got := LaunchSettingsChanged(launch, rewritten); len(got) != 0 {
		t.Fatalf("an empty list rewritten as absent reported %v", got)
	}
	if got := LaunchSettingsChanged(UserSettings{}, UserSettings{Sandbox: &ExecSandboxSettings{}}); len(got) != 0 {
		t.Fatalf("an empty sandbox section, off either way, reported %v", got)
	}
	changed := UserSettings{MaxSessionUSD: 2, TurnToolCalls: 64, ReviewerModel: "local/other"}
	if got := LaunchSettingsChanged(UserSettings{}, changed); !slices.Equal(got, []string{"max_session_usd", "turn_tool_calls", "reviewer_model"}) {
		t.Fatalf("changed = %v", got)
	}
}
