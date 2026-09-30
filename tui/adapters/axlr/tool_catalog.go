package axlr

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/plugins"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// ToolCatalog discovers tools through the same manager used by the executor.
// Each returned slice belongs to its caller and is the lookup for one turn.
type ToolCatalog struct {
	Plugins     *plugins.Manager
	Diagnostics application.DiagnosticPort
	Profiles    func() []domain.PluginProfile
}

var portableName = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func (c ToolCatalog) Snapshot(ctx context.Context) ([]domain.AvailableTool, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := localToolDefinitions()
	result = append(result, application.HostTools()...)
	if c.Plugins != nil {
		var tools []root.PluginTool
		if c.Profiles == nil {
			var err error
			tools, err = c.Plugins.List(ctx)
			if err != nil {
				return nil, err
			}
		} else {
			profiles := c.Profiles()
			for _, profile := range profiles {
				serverCtx, span := application.StartDiagnosticSpan(ctx, c.Diagnostics, application.DiagnosticActionPluginDiscovery, application.DiagnosticEvent{PluginOrdinal: pluginOrdinal(profiles, profile.ID)})
				serverTools, err := c.Plugins.ListServer(serverCtx, profile.ID)
				span.End(pluginDiagnosticError(errors.Join(err, ctx.Err())))
				if err != nil {
					return nil, err
				}
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				tools = append(tools, serverTools...)
			}
		}
		nativeCounts := map[string]int{}
		for _, tool := range result {
			nativeCounts[string(tool.Definition.Name)]++
		}
		for _, tool := range tools {
			nativeCounts[tool.Ref.ToolName.String()]++
		}
		for _, tool := range tools {
			id, err := domain.NewPluginToolIdentity(tool.Ref)
			if err != nil {
				return nil, err
			}
			// A JSON array is an unambiguous, stable encoding of the exact pair.
			pair, _ := json.Marshal([2]string{tool.Ref.PluginID.String(), tool.Ref.ToolName.String()})
			digest := sha256.Sum256(pair)
			name := root.ToolName("mcp_" + hex.EncodeToString(digest[:24]))
			native := tool.Ref.ToolName.String()
			// Native names make schema/guide instructions actionable. Colliding or
			// nonportable names retain opaque, unambiguous aliases; identity is
			// always resolved from the frozen snapshot, never parsed from a name.
			if portableName.MatchString(native) && nativeCounts[native] == 1 && !strings.HasPrefix(native, "mcp_") {
				name = root.ToolName(native)
			}
			result = append(result, domain.AvailableTool{Identity: id, Definition: root.ToolDefinition{Name: name, Description: root.Text(tool.Ref.PluginID.String() + "/" + tool.Ref.ToolName.String() + ": " + tool.Description), Parameters: tool.InputSchema}})
		}
	}
	if err := validateSnapshot(result); err != nil {
		return nil, err
	}
	return result, nil
}

// ResolveTool uses only the supplied turn snapshot; names never encode authority.
func ResolveTool(snapshot []domain.AvailableTool, name root.ToolName) (domain.ToolIdentity, error) {
	if err := validateSnapshot(snapshot); err != nil {
		return domain.ToolIdentity{}, err
	}
	for _, tool := range snapshot {
		if tool.Definition.Name == name {
			return tool.Identity, nil
		}
	}
	return domain.ToolIdentity{}, fmt.Errorf("unknown model function %q", name)
}
func validateSnapshot(snapshot []domain.AvailableTool) error {
	seen := map[root.ToolName]bool{}
	for _, tool := range snapshot {
		name := tool.Definition.Name
		if !portableName.MatchString(string(name)) || seen[name] {
			return fmt.Errorf("invalid or colliding tool alias %q", name)
		}
		seen[name] = true
		if err := tool.Identity.Validate(); err != nil {
			return err
		}
		if _, err := root.NewJSONObject(tool.Definition.Parameters.Bytes()); err != nil {
			return err
		}
		if _, err := root.NewText(string(tool.Definition.Description)); err != nil {
			return err
		}
	}
	return nil
}
