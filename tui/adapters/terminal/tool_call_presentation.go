package terminal

import (
	"encoding/json"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// Presentation of historical wrappers is not an authorization decision.
func presentationName(call root.ToolCall) root.ToolName {
	if call.Name == application.HostCallToolName {
		var wrapper struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(call.Arguments.Bytes(), &wrapper) == nil && wrapper.Name != "" {
			return root.ToolName(wrapper.Name)
		}
	}
	return call.Name
}
func toolCallPresentation(s domain.SessionState, call root.ToolCall) (string, bool) {
	return toolPresentation(s, presentationName(call))
}
