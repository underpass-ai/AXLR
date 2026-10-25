package domain

// ArtifactSource says how an artifact was delivered.
type ArtifactSource string

const (
	// ArtifactFile is a workspace file delivered as it is.
	ArtifactFile ArtifactSource = "file"
	// ArtifactInline is content the call carried, written under the
	// deliverables folder.
	ArtifactInline ArtifactSource = "inline"
)
