package storage

import "reflect"

// LaunchSettingsChanged names the settings.json keys that the console reads
// only at launch and that differ between the launch and now: local_models,
// context_tokens, prompt_tokens, exec_sandbox, max_session_usd,
// turn_tool_calls and reviewer_model. An edit made in another editor while
// the console runs takes effect only after a restart, and on 20 November
// 2026 a window raised to 90000 tokens in nvim beside a running console said
// nothing about it. The console's own writes (autonomy, favourites, the
// default model) touch none of these keys. An empty list is the absent one
// the console's writes leave (omitempty), and an empty sandbox section is
// off as an absent one is: neither is a change.
func LaunchSettingsChanged(launch, now UserSettings) []string {
	var changed []string
	if (len(launch.LocalModels) > 0 || len(now.LocalModels) > 0) && !reflect.DeepEqual(launch.LocalModels, now.LocalModels) {
		changed = append(changed, "local_models")
	}
	if launch.ContextTokens != now.ContextTokens {
		changed = append(changed, "context_tokens")
	}
	if launch.PromptTokens != now.PromptTokens {
		changed = append(changed, "prompt_tokens")
	}
	if !reflect.DeepEqual(effectiveSandbox(launch), effectiveSandbox(now)) {
		changed = append(changed, "exec_sandbox")
	}
	if launch.MaxSessionUSD != now.MaxSessionUSD {
		changed = append(changed, "max_session_usd")
	}
	if launch.TurnToolCalls != now.TurnToolCalls {
		changed = append(changed, "turn_tool_calls")
	}
	if launch.ReviewerModel != now.ReviewerModel {
		changed = append(changed, "reviewer_model")
	}
	return changed
}

// effectiveSandbox is the sandbox section as the console applies it, an
// empty writable list as an absent one.
func effectiveSandbox(s UserSettings) ExecSandboxSettings {
	sandbox := s.ExecSandbox()
	if len(sandbox.Writable) == 0 {
		sandbox.Writable = nil
	}
	return sandbox
}
