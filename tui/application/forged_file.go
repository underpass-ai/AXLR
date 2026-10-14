package application

// ForgedFile is one file of a tool, its path relative to the tool's directory.
type ForgedFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}
