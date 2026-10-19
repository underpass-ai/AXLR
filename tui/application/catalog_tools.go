package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// The model's two catalog tools: axlr_catalog looks in the catalogs before
// it forges, without a card, and axlr_install_tool installs one on the
// install card, approved as a local write. Both are fixed definitions, so
// they keep the request prefix like the forge tools.
const (
	HostCatalogName     root.ToolName = "axlr_catalog"
	HostInstallToolName root.ToolName = "axlr_install_tool"
)

// CatalogTool is axlr_catalog.
func CatalogTool() domain.AvailableTool {
	identity, _ := domain.NewHostToolIdentity(domain.HostOperationCatalog)
	schema, _ := root.NewJSONObject([]byte(`{"type":"object","properties":{"query":{"type":"string","maxLength":512},"name":{"type":"string","pattern":"^[a-z][a-z0-9_]{1,47}$"},"catalog":{"type":"string","pattern":"^[a-z][a-z0-9_-]{0,31}$"},"limit":{"type":"integer","minimum":1,"maximum":20}},"additionalProperties":false}`))
	return domain.AvailableTool{Identity: identity, Definition: root.ToolDefinition{
		Name:        HostCatalogName,
		Description: "Search the catalogs of pre-forged, tested tools before forging one: query returns the tools whose name, tags, category or summary contain every term (empty: all), each with its catalog, version and whether this workspace has it installed; name returns one tool's schema, command, files, requirements and licence. A match is installed with axlr_install_tool, then run with axlr_run_tool. Reads the cached signed index only; nothing is installed and no card is shown.",
		Parameters:  schema,
	}}
}

// InstallTool is axlr_install_tool.
func InstallTool() domain.AvailableTool {
	identity, _ := domain.NewHostToolIdentity(domain.HostOperationInstallTool)
	schema, _ := root.NewJSONObject([]byte(`{"type":"object","properties":{"name":{"type":"string","pattern":"^[a-z][a-z0-9_]{1,47}$"},"version":{"type":"string","pattern":"^\\d+\\.\\d+\\.\\d+$"},"catalog":{"type":"string","pattern":"^[a-z][a-z0-9_-]{0,31}$"}},"required":["name"],"additionalProperties":false}`))
	return domain.AvailableTool{Identity: identity, Definition: root.ToolDefinition{
		Name:        HostInstallToolName,
		Description: "Install a tool of a catalog (found with axlr_catalog) into this workspace as a forged tool: the console fetches its files, verifies them against the catalog's signed index and shows them on the install card; on approval they are written under " + ForgedToolsDir + "/<name>/ and the tool is registered with its origin, callable at once with axlr_run_tool. version defaults to the newest and catalog to the only one that has the tool. Do not forge a copy of a catalog tool instead. Approved like local_write.",
		Parameters:  schema,
	}}
}

// hostCatalog is axlr_catalog: a search over the cached indexes or one
// tool's description.
func (u HostToolUseCase) hostCatalog(ctx context.Context, session domain.Session, arguments root.JSONValue) (any, error) {
	if u.Catalog == nil || u.Forge == nil {
		return nil, errors.New("tool catalogs are not enabled in this console")
	}
	var args struct {
		Query   *string `json:"query"`
		Name    string  `json:"name"`
		Catalog string  `json:"catalog"`
		Limit   *int    `json:"limit"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(arguments.Bytes())))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&args); err != nil {
		return nil, errors.New("axlr_catalog arguments must match its schema: query, name, catalog, limit")
	}
	if args.Catalog != "" && !catalogName.MatchString(args.Catalog) {
		return nil, errors.New("catalog must be a configured catalog's name")
	}
	limit := 8
	if args.Limit != nil {
		if *args.Limit < 1 || *args.Limit > 20 {
			return nil, errors.New("limit must be an integer between 1 and 20")
		}
		limit = *args.Limit
	}
	use := CatalogUseCase{Catalog: u.Catalog, Forge: u.Forge}
	workspace := string(session.Export().Workspace)
	if args.Name != "" {
		if args.Query != nil {
			return nil, errors.New("use name for one tool or query for a search, not both")
		}
		if !forgedToolName.MatchString(args.Name) {
			return nil, errors.New("name must be 2-48 lowercase letters, digits or underscores, starting with a letter")
		}
		match, err := use.Describe(ctx, workspace, InstallRequest{Name: args.Name, Catalog: args.Catalog})
		if err != nil {
			return nil, err
		}
		return catalogToolView(match), nil
	}
	query := ""
	if args.Query != nil {
		query = *args.Query
	}
	matches, catalogs, err := use.Search(ctx, workspace, strings.Fields(query))
	if err != nil {
		return nil, err
	}
	summaries := make([]map[string]any, 0, limit)
	for _, match := range matches {
		if args.Catalog != "" && match.Catalog != args.Catalog {
			continue
		}
		if len(summaries) == limit {
			break
		}
		summary := map[string]any{"name": match.Tool.Name, "version": match.Tool.Version, "catalog": match.Catalog, "category": match.Tool.Category, "summary": match.Tool.Summary, "program": match.Tool.Program, "install_with": HostInstallToolName}
		if match.InstalledVersion != "" {
			summary["installed"] = match.InstalledVersion
		}
		summaries = append(summaries, summary)
	}
	states := make([]map[string]any, 0, len(catalogs))
	for _, state := range catalogs {
		item := map[string]any{"name": state.Name}
		if state.Index != nil {
			item["generated_at"], item["tools"] = state.Index.GeneratedAt, len(NewestOfEach(state.Index.Tools))
		}
		if state.Warning != "" {
			item["warning"] = state.Warning
		}
		states = append(states, item)
	}
	return map[string]any{"tools": summaries, "total_matches": len(matches), "catalogs": states}, nil
}

// catalogToolView is axlr_catalog's answer for one tool.
func catalogToolView(match CatalogMatch) map[string]any {
	tool := match.Tool
	files := make([]string, 0, len(tool.Files))
	for file := range tool.Files {
		files = append(files, file)
	}
	sort.Strings(files)
	view := map[string]any{"name": tool.Name, "version": tool.Version, "catalog": match.Catalog, "category": tool.Category, "tags": tool.Tags, "summary": tool.Summary, "program": tool.Program, "requires": tool.Requires, "network": tool.Network, "license": tool.License, "files": files, "install_with": HostInstallToolName, "call_with": HostRunToolName}
	if match.InstalledVersion != "" {
		view["installed"] = match.InstalledVersion
	}
	return view
}

// installCardKey carries the install card the person approved.
type installCardKey struct{}

// installCard is what an axlr_install_tool card showed: the call it was
// for and the staged tool on it.
type installCard struct {
	call   root.ToolCallID
	staged StagedTool
}

// WithInstallCard carries the staged tool the card of call showed to its
// approval, so the install writes that tool and no other. On 20 October
// 2026 a review found that the approved call staged the tool again from
// its arguments: the catalog cache is shared by every console, and
// another console's /tools or axlr_catalog could refresh it between the
// card and the key press, so a call without a version installed a newer
// one than the card showed.
func WithInstallCard(ctx context.Context, call root.ToolCallID, staged StagedTool) context.Context {
	return context.WithValue(ctx, installCardKey{}, installCard{call: call, staged: staged})
}

// shownInstall is the staged tool the card of the call being executed
// showed, if the person approved that call on its card. An approval that
// covers several calls carries the card of the first alone.
func shownInstall(ctx context.Context) (StagedTool, bool) {
	card, ok := ctx.Value(installCardKey{}).(installCard)
	call, executing := ToolCallOf(ctx)
	if !ok || !executing || card.call != call {
		return StagedTool{}, false
	}
	return card.staged, true
}

// sameStagedTool reports whether two stagings are the same tool: the
// catalog, name and version, and the digests the signed index gives its
// manifest and files.
func sameStagedTool(a, b StagedTool) bool {
	if a.Key() != b.Key() || a.Entry.ManifestSHA256 != b.Entry.ManifestSHA256 || len(a.Entry.Files) != len(b.Entry.Files) {
		return false
	}
	for name, digest := range a.Entry.Files {
		if other, listed := b.Entry.Files[name]; !listed || !strings.EqualFold(other, digest) {
			return false
		}
	}
	return true
}

// hostInstall is axlr_install_tool: the approved call stages the tool
// again (the collision checks and the digests hold at the time of the
// write) and registers it. Approved on its card, the call is pinned to
// the catalog and version the card showed, and the tool staged again must
// be the one on the card; otherwise nothing is installed.
func (u HostToolUseCase) hostInstall(ctx context.Context, session domain.Session, arguments root.JSONValue) (any, error) {
	if u.Catalog == nil || u.Forge == nil {
		return nil, errors.New("tool catalogs are not enabled in this console")
	}
	request, err := decodeInstallRequest(arguments)
	if err != nil {
		return nil, err
	}
	card, shown := shownInstall(ctx)
	if shown {
		if card.Entry.Name != request.Name {
			return nil, fmt.Errorf("the card showed %s, not %s; nothing was installed", card.Key(), request.Name)
		}
		request.Catalog, request.Version = card.Catalog, card.Entry.Version
	}
	use := CatalogUseCase{Catalog: u.Catalog, Forge: u.Forge}
	state := session.Export()
	staged, err := use.Stage(ctx, string(state.Workspace), request)
	if err != nil {
		if shown {
			// The version may be gone from a refreshed index, or a tool
			// of the same name arrived meanwhile.
			return nil, fmt.Errorf("the card showed %s, which cannot be installed now: %v; nothing was installed; call %s again for a new card", card.Key(), err, HostInstallToolName)
		}
		return nil, err
	}
	if shown && !sameStagedTool(card, staged) {
		return nil, fmt.Errorf("the catalog changed since the card showed %s: it now has different files for it; nothing was installed; call %s again for a new card", card.Key(), HostInstallToolName)
	}
	stored, replaced, err := use.Install(ctx, string(state.Workspace), staged, string(state.ID))
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(staged.Files))
	for _, file := range staged.Files {
		paths = append(paths, ForgedToolsDir+"/"+stored.Name+"/"+file.Path)
	}
	result := map[string]any{"installed": stored.Name, "version": staged.Entry.Version, "catalog": staged.Catalog, "replaced": replaced, "files": paths, "command": append([]string{stored.Program}, stored.Args...), "call_with": string(HostRunToolName)}
	if len(staged.Warnings) > 0 {
		result["warnings"] = staged.Warnings
	}
	return result, nil
}

// decodeInstallRequest reads axlr_install_tool's arguments.
func decodeInstallRequest(arguments root.JSONValue) (InstallRequest, error) {
	var args struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Catalog string `json:"catalog"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(arguments.Bytes())))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&args); err != nil || args.Name == "" {
		return InstallRequest{}, errors.New("axlr_install_tool arguments must be name, with version and catalog optional")
	}
	if !forgedToolName.MatchString(args.Name) {
		return InstallRequest{}, fmt.Errorf("unknown catalog tool %q; axlr_catalog lists the catalog tools", args.Name)
	}
	return InstallRequest{Name: args.Name, Version: args.Version, Catalog: args.Catalog}, nil
}

// InstallRequestOf reads the request of a pending axlr_install_tool call,
// for the card that stages it; ok is false for other arguments.
func InstallRequestOf(arguments []byte) (InstallRequest, bool) {
	value, err := root.NewJSONObject(arguments)
	if err != nil {
		return InstallRequest{}, false
	}
	request, err := decodeInstallRequest(StripWhy(value))
	return request, err == nil
}
