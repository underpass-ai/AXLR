package domain

// DeliveredArtifact is the result axlr_deliver returns to the model, and
// the record the result reads its artifact from: the artifact without its
// session and call, which the call's record holds.
type DeliveredArtifact struct {
	Artifact DeliveredArtifactFields `json:"artifact"`
}

// DeliveredArtifactFields are an artifact's fields in axlr_deliver's
// result.
type DeliveredArtifactFields struct {
	Name        string         `json:"name"`
	MediaType   string         `json:"media_type"`
	Description string         `json:"description"`
	Size        int64          `json:"size"`
	SHA256      string         `json:"sha256"`
	Reference   string         `json:"reference"`
	Source      ArtifactSource `json:"source"`
}
