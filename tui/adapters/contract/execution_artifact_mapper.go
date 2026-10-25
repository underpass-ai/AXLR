package contract

import (
	"github.com/underpass-ai/AXLR/tui/domain"
	"github.com/underpass-ai/AXLR/tui/dto"
)

// ExecutionArtifact is the wire form of an artifact.
func ExecutionArtifact(artifact domain.ExecutionArtifact) dto.ExecutionArtifact {
	return dto.ExecutionArtifact{Name: artifact.Name, MediaType: artifact.MediaType, Size: artifact.Size, SHA256: artifact.SHA256, Reference: artifact.Reference, Description: artifact.Description, Source: string(artifact.Source), Session: string(artifact.Session), Call: string(artifact.Call)}
}
