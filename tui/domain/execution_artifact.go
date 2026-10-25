package domain

import (
	"encoding/json"

	axlr "github.com/underpass-ai/AXLR/domain"
)

// ExecutionArtifact is one deliverable an execution handed over with
// axlr_deliver (the runtime contract's artifacts): what it is, the bytes
// delivered and where the control plane fetches them in the workspace.
type ExecutionArtifact struct {
	Name        string
	MediaType   string
	Description string
	Size        int64
	// SHA256 is the hex digest of the bytes delivered, taken when the call
	// ran.
	SHA256 string
	// Reference is the file's path relative to the workspace, with /
	// separators.
	Reference string
	Source    ArtifactSource
	Session   SessionID
	Call      axlr.ToolCallID
}

// ArtifactOf reads the artifact a completed axlr_deliver call delivered in
// session; false for any other call, or one that failed or did not run.
func ArtifactOf(session SessionID, record PendingTool) (ExecutionArtifact, bool) {
	if record.Call.Name != DeliverToolName || record.Outcome == nil || record.Outcome.IsError || record.Outcome.Uncertain || (record.Decision != DecisionApprove && record.Decision != DecisionAutoApprove) {
		return ExecutionArtifact{}, false
	}
	var delivered DeliveredArtifact
	if json.Unmarshal([]byte(record.Outcome.Content), &delivered) != nil {
		return ExecutionArtifact{}, false
	}
	fields := delivered.Artifact
	if fields.Reference == "" || len(fields.SHA256) != 64 || (fields.Source != ArtifactFile && fields.Source != ArtifactInline) {
		return ExecutionArtifact{}, false
	}
	return ExecutionArtifact{Name: fields.Name, MediaType: fields.MediaType, Description: fields.Description, Size: fields.Size, SHA256: fields.SHA256, Reference: fields.Reference, Source: fields.Source, Session: session, Call: record.Call.ID}, true
}

// SessionArtifacts are the artifacts the session's calls delivered, in
// delivery order, a later delivery of the same reference replacing the
// earlier one.
func SessionArtifacts(s Session) []ExecutionArtifact {
	state := s.Export()
	var artifacts []ExecutionArtifact
	for _, record := range state.Activity {
		if artifact, ok := ArtifactOf(state.ID, record); ok {
			artifacts = withArtifact(artifacts, artifact)
		}
	}
	return artifacts
}

// withArtifact appends artifact, dropping an earlier delivery of its
// reference: the file now holds the later bytes.
func withArtifact(artifacts []ExecutionArtifact, artifact ExecutionArtifact) []ExecutionArtifact {
	out := artifacts[:0:0]
	for _, earlier := range artifacts {
		if earlier.Reference != artifact.Reference {
			out = append(out, earlier)
		}
	}
	return append(out, artifact)
}
