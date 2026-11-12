package application

import (
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// ToolCallRecord is one finished tool call as ToolCallLogPort hears it.
// It has no field for the arguments on purpose.
type ToolCallRecord struct {
	Session domain.SessionID
	Call    root.ToolCallID
	// Tool is the name the model called; PluginTool is the plugin's tool
	// it reached (plugin/tool), empty for the console's own tools and for
	// a call that does not resolve.
	Tool       root.ToolName
	PluginTool string
	Decision   domain.ToolDecision
	// Failed says the call's outcome is a failure; Error is then that
	// outcome's text, flattened to one line and at most
	// MaxToolCallErrorBytes (ToolCallErrorText).
	Failed bool
	Error  string
	// Duration runs from the decision to the outcome; zero for a call the
	// console refused before it ran.
	Duration time.Duration
}
