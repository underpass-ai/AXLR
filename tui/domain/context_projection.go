package domain

import root "github.com/underpass-ai/AXLR/domain"

// ContextProjection reports a disposable model view of the original transcript.
// CutIndex and retrieval references use absolute, zero-based original indices.
type ContextProjection struct {
	Messages         []root.Message
	OriginalMessages int
	DroppedMessages  int
	OriginalBytes    int
	ProjectedBytes   int
	CutIndex         int
	// TurnCompacted is true when the turn in progress did not fit and its
	// tool results were shortened beyond the normal per-result limit.
	TurnCompacted bool
}
