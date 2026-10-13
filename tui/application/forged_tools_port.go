package application

import "context"

// ForgedToolsPort keeps a workspace's forged tools. Only Forge registers a
// tool: a file written another way into ForgedToolsDir, or brought by a
// checkout, is never one, and Verify refuses a tool whose files changed
// since it was forged.
type ForgedToolsPort interface {
	List(ctx context.Context, workspace string) ([]ForgedTool, error)
	// Forge replaces the tool's directory with files, fills Files and
	// ForgedAt and registers the tool; replaced reports a previous version.
	Forge(ctx context.Context, workspace string, tool ForgedTool, files []ForgedFile) (stored ForgedTool, replaced bool, err error)
	Verify(ctx context.Context, workspace string, tool ForgedTool) error
	// Remove deletes a registered tool's directory and unregisters it; it
	// does not verify the files first, so a tool whose files are already
	// gone can still be removed. found is false, and nothing is deleted,
	// for a name that is not registered.
	Remove(ctx context.Context, workspace, name string) (found bool, err error)
}
