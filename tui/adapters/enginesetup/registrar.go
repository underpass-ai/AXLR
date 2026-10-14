package enginesetup

import (
	"context"

	"github.com/underpass-ai/AXLR/tui/application"
)

// Registrar writes the mcp.json entry of an installed engine.
type Registrar interface {
	RegisterEngine(context.Context, application.EngineRegistration) error
}
