package dto

// ExecutionArtifact is one deliverable of an execution, in its result and
// its artifact event: what it is, the bytes delivered and the workspace
// path the control plane fetches them from.
type ExecutionArtifact struct {
	Name        string `json:"name"`
	MediaType   string `json:"media_type"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	Reference   string `json:"reference"`
	Description string `json:"description"`
	Source      string `json:"source"`
	Session     string `json:"session_id"`
	Call        string `json:"call_id"`
}
