package kmpstore

// Selection is the store of one console launch.
type Selection struct {
	Path   string
	Reason Reason
	// TopLevel is the git checkout's top level for a project store.
	TopLevel string
	// Replaced is the installer's old per-user pin the project store
	// overrides, empty when there was none.
	Replaced string
}
