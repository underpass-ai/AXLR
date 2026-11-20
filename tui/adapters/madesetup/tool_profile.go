package madesetup

import (
	"path/filepath"
	"strings"

	"github.com/underpass-ai/AXLR/plugins"
)

// ToolProfileVariable is the MADE launcher setting that chooses the tools the
// server lists and accepts.
const ToolProfileVariable = "MADE_MCP_TOOL_PROFILE"

// ToolProfile is the profile AXLR runs MADE under. Since MADE 0.11 the
// embedded launcher defaults to `core`, which hides grant administration and
// the resume inspection, host handoff and agent status the console calls on
// claimed turns: on 20 November 2026 the work grant failed on every start of a
// fresh install (wrx80), so its sessions never had a ceremony instance, and
// every install refused the three calls. The model never receives MADE's
// schemas (only local and host tools are sent), so the full catalog costs no
// prompt.
const ToolProfile = "full"

// WithToolProfile is env with MADE's tool profile set to ToolProfile, unless
// the entry names a profile itself: an operator's choice stays.
func WithToolProfile(env []string) []string {
	for _, entry := range env {
		if strings.HasPrefix(entry, ToolProfileVariable+"=") {
			return env
		}
	}
	return append(append([]string(nil), env...), ToolProfileVariable+"="+ToolProfile)
}

// WithWorkIdentity is the environment of a MADE entry with the console's work
// identity pinned when the entry is the embedded launcher and names none: the
// identity the preparation grants and then saves in the entry. Pinned at
// launch, the engine runs as it from the start and sees the grant issued a
// moment later on its next call; until 20 November 2026 it ran as the store's
// trusted host, which holds no grant, so a fresh install's first console had
// no session instance until it was restarted.
func WithWorkIdentity(configPath string, r plugins.Registration) []string {
	values := environment(r.Env)
	if r.Manifest.URL != "" || values["MADE_MCP_BACKEND"] == "grpc" || filepath.Base(r.Manifest.Command) != embeddedLauncher || values[hostIdentityKey] != "" {
		return r.Env
	}
	work := WorkIdentity(configPath, r.Env)
	if work == "" {
		return r.Env
	}
	return append(append([]string(nil), r.Env...), hostIdentityKey+"="+work)
}
