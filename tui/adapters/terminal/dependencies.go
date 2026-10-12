package terminal

import (
	"context"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type Dependencies struct {
	Context           context.Context
	Diagnostics       application.DiagnosticPort
	Plugins           application.PluginManagementPort
	InstalledPlugins  application.InstalledPluginPort
	EngineUpdates     application.EngineUpdatePort
	MADEPreparation   application.MADEPreparationPort
	Models            application.ListModelsUseCase
	ModelPreference   application.ModelPreferencePort
	SessionLabels     application.SessionLabelsPort
	ModelFavorites    application.ModelFavoritesPort
	UIPreferenceStore application.UIPreferencePort
	UIPreferences     domain.UIPreferences
	ApprovalSettings  application.ApprovalSettingsPort
	Locale            Locale
	Create            application.CreateSessionUseCase
	Change            application.ChangeSessionModelUseCase
	Workspace         domain.Workspace
	NewSessionID      domain.SessionID
	Start             application.StartTurnUseCase
	Resolve           application.ResolveToolUseCase
	Agent             application.AgentTurnUseCase
	Store             application.SessionStorePort
	// ExportFiles writes /export's Markdown; nil refuses /export.
	ExportFiles application.ExportFilePort
	// DeleteSession removes a saved session from Ctrl+O; nil offers no
	// delete.
	DeleteSession application.SessionDeletePort
	Session       *domain.Session
	Monochrome    bool
	// InitialDraft fills the composer at launch, such as the repair brief;
	// the person still presses Enter.
	InitialDraft string
	// Repairs shows the self-repairs the agent requested; nil without MADE.
	Repairs RepairPanelPort
	// Plans shows the plans and their tasks; nil without MADE.
	Plans PlanPanelPort
	// Usage is the session's usage ledger for the footer's cost and
	// /usage, where + raises the session's limit by MaxSessionUSD.
	Usage         application.SessionUsagePort
	MaxSessionUSD float64
	// PromptCeiling is the prompt, in tokens, a request to a model may
	// reach, for the footer's "27k/64k"; nil, or zero, shows the tokens
	// alone.
	PromptCeiling func(root.ModelID) int
	// Sandbox is the exec sandbox the launch found; the header marks one
	// that is configured but does not confine commands.
	Sandbox SandboxStatus
	// Quit receives the session shown when the person quits (/exit,
	// Ctrl+C), so the launcher can say how to resume it.
	Quit func(domain.SessionState)
}
