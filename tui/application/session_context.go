package application

import (
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type sessionContext struct {
	ID          domain.SessionID `json:"session_id"`
	Workspace   domain.Workspace `json:"workspace"`
	PromptCount int              `json:"user_prompt_count"`
	Title       root.Text        `json:"title"`
	// About is the about the console uses for the session's memory: the
	// selected one, or by default the workspace's project about, which
	// AboutIsDefault marks.
	About          string `json:"about"`
	AboutIsDefault bool   `json:"about_is_default,omitempty"`
	// TitleDeferred says why a proposed title was not stored while the about
	// sent with it was.
	TitleDeferred string `json:"title_deferred,omitempty"`
}
