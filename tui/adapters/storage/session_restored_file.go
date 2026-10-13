package storage

type sessionRestoredFile struct {
	Path string `json:"path"`
	From string `json:"from"`
	To   string `json:"to"`
}
