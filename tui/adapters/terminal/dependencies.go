package terminal

import (
	"context"

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
	Search            application.SearchSessionUseCase
	Store             application.SessionStorePort
	Session           *domain.Session
	Monochrome        bool
	// InitialDraft fills the composer at launch, such as the repair brief;
	// the person still presses Enter.
	InitialDraft string
	// Repairs shows the self-repairs the agent requested; nil without MADE.
	Repairs RepairPanelPort
}
