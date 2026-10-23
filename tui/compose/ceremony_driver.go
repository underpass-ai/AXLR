package compose

import (
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/tui/adapters/ceremonyhost"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/application"
)

// CeremonyDriver drives /debug, /delivery and /incident when MADE is
// connected; nil without a made registration. KMP is optional: without it
// ceremonies run without memory. owner is the lease owner of its claims,
// "" for the engine adapter's default.
func CeremonyDriver(registrations []plugins.Registration, tools application.ToolExecutionPort, labels application.SessionLabelsPort, owner string) *application.CeremonyDriver {
	connected := map[string]bool{}
	for _, registration := range registrations {
		connected[registration.Manifest.ID.String()] = true
	}
	if !connected["made"] {
		return nil
	}
	engine := ceremonyhost.Engine{Tools: tools, Owner: owner}
	driver := &application.CeremonyDriver{Engine: engine, Definitions: engine, Checks: ceremonyhost.Checks{Tools: tools}, Labels: labels}
	if connected["kmp"] {
		driver.Memory = ceremonyhost.Memory{Tools: tools}
	}
	return driver
}

// CompactProfile decides the ceremony profile per session model: always or
// never when settings say so, otherwise compact for a known window of at
// most storage.CompactWindowTokens.
func CompactProfile(profile string, windows application.ModelContextWindowPort) func(root.ModelID) bool {
	switch profile {
	case "compact":
		return func(root.ModelID) bool { return true }
	case "standard":
		return nil
	}
	return func(model root.ModelID) bool {
		window := windows.ContextWindow(model)
		return window > 0 && window.Tokens() <= storage.CompactWindowTokens
	}
}
