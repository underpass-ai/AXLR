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
	Fresh bool
	// Published names the definition /ceremony publish published.
	Published string
	// Ceremonies are the definitions /ceremony listed; CeremonyArm, when
	// set, is the definition to arm with them.
	Ceremonies  *[]application.CeremonyDefinitionSummary
	CeremonyArm *ceremonyArm
	// MemorySaved, MemoryCompared and MemoryForgotten are /memory's
	// results: the bundle written, the comparison read, the entry closed.
	MemorySaved     *application.BundleSaved
	MemoryCompared  *application.CompareResult
	MemoryForgotten *application.ForgetReceipt
	Session         domain.Session
	Err             error
	Sessions        *[]domain.SessionSummary
	// Held marks the listed sessions another console has open.
	Held                    map[domain.SessionID]bool
	Plugins                 *[]domain.PluginState
	InstalledPlugins        *[]application.InstalledPlugin
	PluginApproval          *domain.PluginProfile
	EngineUpdates           *[]application.EngineUpdateResult
	OrchestratorPreparation *application.OrchestratorPreparation
	// gitRead marks a turn's end whose Git state read is already asked.
	gitRead bool
}
