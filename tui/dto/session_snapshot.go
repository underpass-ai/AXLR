package dto

// SessionSnapshot is the versioned on-disk schema. Credentials are not session state.
type SessionSnapshot struct {
	Version        int             `json:"version"`
	ID             string          `json:"id"`
	Workspace      string          `json:"workspace"`
	Model          string          `json:"model"`
	Status         string          `json:"status"`
	Messages       []Message       `json:"messages"`
	ToolSnapshot   []AvailableTool `json:"tool_snapshot"`
	Activity       []ToolActivity  `json:"activity"`
	ArchivedDrafts []ArchivedDraft `json:"archived_drafts,omitempty"`
	Draft          string          `json:"draft"`
	TurnCallCount  int             `json:"turn_call_count"`
}
