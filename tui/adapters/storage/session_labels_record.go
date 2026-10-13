package storage

type sessionLabelsRecord struct {
	Version int                          `json:"version"`
	Labels  map[string]sessionLabelEntry `json:"labels"`
}
