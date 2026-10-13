package application

import root "github.com/underpass-ai/AXLR/domain"

// EngineGuidancePort words the model's orientation for one registered
// plugin: what the console does with the engine and what the model never
// does (decision D8 of the lifecycle integration design). The application
// knows the plugin's role, not its name; the adapter that registered the
// plugin knows both. An empty paragraph says nothing about the plugin.
type EngineGuidancePort interface {
	Paragraph(plugin root.PluginID, facts PluginGuidanceFacts) string
}
