// Package kmpstore chooses and prepares the KMP store a console opens for a
// workspace: the checkout's own .kernel inside a git repository, the
// per-user default outside one, or the directory an operator pinned in
// mcp.json. A store the console creates receives the repository's committed
// bundle and the KMP guide, so a clone on another machine starts with the
// project's memory (decision D1 of the lifecycle integration design).
package kmpstore

const (
	// DataDirVariable pins the store of a kmp-mcp process.
	DataDirVariable = "KMP_MCP_DATA_DIR"
	// BackendVariable selects the embedded kernel.
	BackendVariable = "KMP_MCP_BACKEND"
	// StoreDirectory is the project store inside a checkout, the one KMP's
	// own precedence chooses for a git repository.
	StoreDirectory = ".kernel"
	// BundlePath is the committed memory of a repository, relative to its
	// top level, as kmp-mcp export maintains it and kmp-mcp import restores it.
	BundlePath = ".kmp/memory.jsonl"
	// Launcher is the basename of a KMP package's embedded launcher; the
	// engine binary sits beside it in bin/.
	Launcher = "run-embedded-mcp.sh"
)
