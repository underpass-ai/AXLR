package terminal

import "github.com/underpass-ai/AXLR/tui/application"

// ceremonyArmMsg arms a definition once the operation that listed the
// definitions has ended.
type ceremonyArmMsg struct {
	arm    ceremonyArm
	listed []application.CeremonyDefinitionSummary
}
