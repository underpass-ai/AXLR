package storage

import "github.com/underpass-ai/AXLR/tui/domain"

type approvalSettingsFile struct {
	Version    int                   `json:"version"`
	Autonomous bool                  `json:"autonomous"`
	Allowed    []domain.ToolIdentity `json:"allowed"`
}
