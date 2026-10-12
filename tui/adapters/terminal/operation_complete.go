package terminal

import (
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type operationComplete struct {
	ID             uint64
	Models         *[]domain.AvailableModel
	ModelSelection bool
	PreferenceErr  error
	// DefaultSaved reports a model selection that also became the default
	// for new sessions.
	DefaultSaved bool
	// Fresh reports the new session /new started.
	Fresh            bool
	Session          domain.Session
	Err              error
	Sessions         *[]domain.SessionSummary
	Plugins          *[]domain.PluginState
	InstalledPlugins *[]application.InstalledPlugin
	PluginApproval   *domain.PluginProfile
	EngineUpdates    *[]application.EngineUpdateResult
	MADEPreparation  *application.MADEPreparation
	// gitRead marks a turn's end whose Git state read is already asked.
	gitRead bool
}
