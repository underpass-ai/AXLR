package application

import (
	"context"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// SuccessorPort opens the session that continues a predecessor: same
// workspace, model, mode and about, with the predecessor named in its
// instance so its memory is recalled.
type SuccessorPort interface {
	Continue(ctx context.Context, predecessor domain.Session) (domain.Session, error)
}
