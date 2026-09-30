package application

import (
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type ModelContextPort interface {
	Project([]root.Message) (domain.ContextProjection, error)
}
