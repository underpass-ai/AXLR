package axlr

import (
	"context"
	"errors"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
	"sort"
)

func orderedPluginProfiles(profiles []domain.PluginProfile) []domain.PluginProfile {
	ordered := append([]domain.PluginProfile(nil), profiles...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID.String() < ordered[j].ID.String() })
	return ordered
}

func pluginOrdinal(profiles []domain.PluginProfile, id root.PluginID) int {
	for i, p := range orderedPluginProfiles(profiles) {
		if p.ID == id {
			return i + 1
		}
	}
	return 0
}

func pluginDiagnosticError(err error) application.DiagnosticErrorClass {
	if errors.Is(err, context.Canceled) {
		return application.DiagnosticErrorCancelled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return application.DiagnosticErrorTimeout
	}
	if err != nil {
		return application.DiagnosticErrorTool
	}
	return application.DiagnosticErrorNone
}
