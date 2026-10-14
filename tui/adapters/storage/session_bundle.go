package storage

import "github.com/underpass-ai/AXLR/tui/dto"

type sessionBundle struct {
	Version    int                 `json:"version"`
	ExportedAt string              `json:"exported_at"`
	Host       string              `json:"host,omitempty"`
	Title      string              `json:"title,omitempty"`
	About      string              `json:"about,omitempty"`
	Snapshot   dto.SessionSnapshot `json:"snapshot"`
	Times      []string            `json:"times,omitempty"`
	Mode       string              `json:"mode,omitempty"`
	Instance   *sessionInstance    `json:"instance,omitempty"`
	Console    *sessionConsole     `json:"console,omitempty"`
}
