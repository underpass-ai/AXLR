package application

import (
	"context"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type PluginManagementPort interface {
	List(context.Context) ([]domain.PluginState, error)
	SetApproval(context.Context, root.PluginID, domain.ApprovalMode) error
	InstallManifest(context.Context, string) error
	InstallURL(context.Context, root.PluginID, string) error
}

// PackageServer is one MCP registration an AXLR package owned, as it was
// persisted before the package was removed or replaced: an update restores
// its approval and the literal environment values the person set.
type PackageServer struct {
	ID       root.PluginID
	Manifest string
	Approval domain.ApprovalMode
	EnvFrom  map[string]string
	Env      map[string]string
}

// PackageServerRemover removes the MCP registrations whose manifests live in
// one package directory, from the persistent configuration first and then
// from the running console.
type PackageServerRemover interface {
	RemovePackageServers(context.Context, string) ([]PackageServer, error)
}
