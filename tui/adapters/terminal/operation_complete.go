package terminal

import (
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type operationComplete struct {
	ID               uint64
	Models           *[]domain.AvailableModel
	ModelSelection   bool
	PreferenceErr    error
	Session          domain.Session
	Err              error
	Sessions         *[]domain.SessionSummary
	Plugins          *[]domain.PluginState
	InstalledPlugins *[]application.InstalledPlugin
	PluginApproval   *domain.PluginProfile
}
