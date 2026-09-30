package domain

import root "github.com/underpass-ai/AXLR/domain"

// PluginState is a safe projection of one profile and its advertised tools.
type PluginState struct {
	Profile PluginProfile
	Tools   []root.PluginToolName
	Error   root.Text
}
