package storage

// sessionRestore names the steps a restore went back over; their
// snapshots stay in the session's activity.
type sessionRestore struct {
	Undo  bool                  `json:"undo,omitempty"`
	Files []sessionRestoredFile `json:"files"`
}
