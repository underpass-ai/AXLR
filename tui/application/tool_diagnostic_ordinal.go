package application

import (
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// toolDiagnosticOrdinal correlates a measured call with the ordered call history
// in private session/request payloads, without putting its raw ID into telemetry.
func toolDiagnosticOrdinal(s *domain.Session, id root.ToolCallID) int {
	if s == nil {
		return 0
	}
	for i, tool := range s.Export().Activity {
		if tool.Call.ID == id {
			return i + 1
		}
	}
	return 0
}
