package application

import "github.com/underpass-ai/AXLR/tui/domain"

// sessionGuidance is the session metadata in the system prompt. It leaves out
// user_prompt_count and the title, which axlr_session still reports: a value
// that changes during the session would change the prompt's prefix, and a
// prefix cache reuses nothing after the first changed token. Setting the
// title broke the cache at message 0 on 8 October 2026; since 10 October the
// console also sets it itself (titleUntitled).
type sessionGuidance struct {
	ID        domain.SessionID `json:"session_id"`
	Workspace domain.Workspace `json:"workspace"`
	About     string           `json:"about"`
}
