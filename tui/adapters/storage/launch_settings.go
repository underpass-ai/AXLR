package storage

import "reflect"

// LaunchSettingsChanged names the settings.json keys that the console reads
// only at launch and that differ between the launch and now: local_models,
// context_tokens, prompt_tokens and exec_sandbox. An edit made in another
// editor while the console runs takes effect only after a restart, and on
// 20 November 2026 a window raised to 90000 tokens in nvim beside a running
// console said nothing about it. The console's own writes (autonomy,
// favourites, the default model) touch none of these keys.
func LaunchSettingsChanged(launch, now UserSettings) []string {
	var changed []string
	if !reflect.DeepEqual(launch.LocalModels, now.LocalModels) {
		changed = append(changed, "local_models")
	}
	if launch.ContextTokens != now.ContextTokens {
		changed = append(changed, "context_tokens")
	}
	if launch.PromptTokens != now.PromptTokens {
		changed = append(changed, "prompt_tokens")
	}
	if !reflect.DeepEqual(launch.Sandbox, now.Sandbox) {
		changed = append(changed, "exec_sandbox")
	}
	return changed
}
