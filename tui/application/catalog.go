package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// A catalog of pre-forged tools (docs/plans/2026-10-16-tool-registry-design.md,
// open mechanisms 1 and 2) is a static, signed index.json plus the files
// of each tool. The console reads the index through CatalogPort, which
// caches it and verifies its minisign signature, and installs a tool as a
// forged tool with an Origin: the same registry, the same axlr_run_tool,
// the same verification of its files at every run.

// CatalogPort reads the configured catalogs.
type CatalogPort interface {
	// Catalogs returns every configured catalog with its cached index. A
	// catalog whose index is older than a day, or every one when refresh
	// is set, is fetched again and verified first; when that fails the
	// old cache stays and Warning says why. Index is nil for a catalog
	// never fetched successfully. The error is the context's only.
	Catalogs(ctx context.Context, refresh bool) ([]CatalogState, error)
	// Cached is Catalogs from the cache alone: a catalog is fetched only
	// when it was never fetched. Staging reads the index this way, so the
	// card and the install that follows resolve the same entry.
	Cached(ctx context.Context) ([]CatalogState, error)
	// Stage fetches a tool's manifest and files for one entry of a cached
	// index, each verified against the digests the signed index carries,
	// and writes nothing into any workspace. It never refreshes the index,
	// so the card and the install that follows read the same entry.
	Stage(ctx context.Context, catalog, name, version string) (StagedTool, error)
}

// CatalogState is one configured catalog as the console sees it.
type CatalogState struct {
	Name      string
	URL       string
	Index     *CatalogIndex
	FetchedAt time.Time
	// Warning is why the last fetch or verification failed, or why the
	// catalog cannot be read (no public key); "" when all is well.
	Warning string
}

// CatalogIndex is index.json (decision D2 of the design); ceremonies are
// kept as they come for the ceremony install that is not built yet.
type CatalogIndex struct {
	SchemaVersion int             `json:"schema_version"`
	Catalog       string          `json:"catalog"`
	GeneratedAt   string          `json:"generated_at"`
	Tools         []CatalogEntry  `json:"tools"`
	Ceremonies    json.RawMessage `json:"ceremonies,omitempty"`
}

// CatalogEntry is one tool of an index.
type CatalogEntry struct {
	Name           string            `json:"name"`
	Version        string            `json:"version"`
	Category       string            `json:"category"`
	Tags           []string          `json:"tags"`
	Summary        string            `json:"summary"`
	Program        string            `json:"program"`
	Requires       CatalogRequires   `json:"requires"`
	Network        bool              `json:"network"`
	License        string            `json:"license"`
	Manifest       string            `json:"manifest"`
	ManifestSHA256 string            `json:"manifest_sha256"`
	Files          map[string]string `json:"files"`
	TestedWith     []string          `json:"tested_with,omitempty"`
	UpdatedAt      string            `json:"updated_at,omitempty"`
}

// CatalogRequires is what a tool needs beside its program.
type CatalogRequires struct {
	ProgramMinVersion string   `json:"program_min_version,omitempty"`
	Packages          []string `json:"packages"`
}

// StagedTool is a catalog tool fetched and verified, ready for its card
// and, once approved, for the registry: Tool is the forged tool it
// becomes (Files is filled by the registry) and Files its contents.
type StagedTool struct {
	Catalog          string
	Entry            CatalogEntry
	IndexGeneratedAt string
	Tool             ForgedTool
	Files            []ForgedFile
	Warnings         []string
}

// Key names a staged tool: the catalog, the tool and the version.
func (s StagedTool) Key() string { return s.Catalog + "/" + s.Entry.Name + "@" + s.Entry.Version }

// InstallRequest asks for a catalog tool: Version "" is the newest and
// Catalog "" the only catalog that has the tool.
type InstallRequest struct {
	Name, Version, Catalog string
}

// CatalogMatch is one tool found in the catalogs, with the version
// installed in the workspace when there is one.
type CatalogMatch struct {
	Catalog          string
	Tool             CatalogEntry
	InstalledVersion string
}

// CatalogOverview is what /tools shows: the workspace's tools, the
// catalogs and the installed catalog tools a catalog has newer.
type CatalogOverview struct {
	Installed []ForgedTool
	Catalogs  []CatalogState
	Updates   []ToolUpdate
}

// ToolUpdate is an installed catalog tool whose catalog has a newer
// version.
type ToolUpdate struct {
	Name, Catalog, Installed, Available string
}

// CatalogUseCase installs catalog tools, for the model (axlr_catalog,
// axlr_install_tool) and for the person (/tools) alike.
type CatalogUseCase struct {
	Catalog CatalogPort
	Forge   ForgedToolsPort
}

var (
	catalogName    = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
	catalogVersion = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
)

// Enabled reports whether both ports are set.
func (u CatalogUseCase) Enabled() bool { return u.Catalog != nil && u.Forge != nil }

// Overview reads the workspace's tools and the catalogs.
func (u CatalogUseCase) Overview(ctx context.Context, workspace string, refresh bool) (CatalogOverview, error) {
	if !u.Enabled() {
		return CatalogOverview{}, errors.New("tool catalogs are not enabled in this console")
	}
	installed, err := u.Forge.List(ctx, workspace)
	if err != nil {
		return CatalogOverview{}, err
	}
	catalogs, err := u.Catalog.Catalogs(ctx, refresh)
	if err != nil {
		return CatalogOverview{}, err
	}
	overview := CatalogOverview{Installed: installed, Catalogs: catalogs}
	for _, tool := range installed {
		if tool.Origin == nil {
			continue
		}
		for _, state := range catalogs {
			if state.Name != tool.Origin.Catalog || state.Index == nil {
				continue
			}
			if latest, found := latestVersion(state.Index.Tools, tool.Name); found && newerVersion(latest.Version, tool.Origin.Version) {
				overview.Updates = append(overview.Updates, ToolUpdate{Name: tool.Name, Catalog: state.Name, Installed: tool.Origin.Version, Available: latest.Version})
			}
		}
	}
	return overview, nil
}

// Search finds the catalog tools whose name, tags, category or summary
// contain every term, every tool when there is none, newest version of
// each name first; a tool installed in the workspace says which version.
func (u CatalogUseCase) Search(ctx context.Context, workspace string, terms []string) ([]CatalogMatch, []CatalogState, error) {
	if !u.Enabled() {
		return nil, nil, errors.New("tool catalogs are not enabled in this console")
	}
	installed, err := u.Forge.List(ctx, workspace)
	if err != nil {
		return nil, nil, err
	}
	catalogs, err := u.Catalog.Catalogs(ctx, false)
	if err != nil {
		return nil, nil, err
	}
	lowered := make([]string, 0, len(terms))
	for _, term := range terms {
		if term = strings.ToLower(strings.TrimSpace(term)); term != "" {
			lowered = append(lowered, term)
		}
	}
	var matches []CatalogMatch
	for _, state := range catalogs {
		if state.Index == nil {
			continue
		}
		for _, tool := range NewestOfEach(state.Index.Tools) {
			text := strings.ToLower(tool.Name + " " + tool.Category + " " + strings.Join(tool.Tags, " ") + " " + tool.Summary)
			if termsMatched(text, lowered) != len(lowered) {
				continue
			}
			match := CatalogMatch{Catalog: state.Name, Tool: tool}
			if existing, found := findForged(installed, tool.Name); found && existing.Origin != nil && existing.Origin.Catalog == state.Name {
				match.InstalledVersion = existing.Origin.Version
			}
			matches = append(matches, match)
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Tool.Name != matches[j].Tool.Name {
			return matches[i].Tool.Name < matches[j].Tool.Name
		}
		return matches[i].Catalog < matches[j].Catalog
	})
	return matches, catalogs, nil
}

// Describe finds one tool by name, in the catalog named or in the only
// catalog that has it.
func (u CatalogUseCase) Describe(ctx context.Context, workspace string, request InstallRequest) (CatalogMatch, error) {
	if !u.Enabled() {
		return CatalogMatch{}, errors.New("tool catalogs are not enabled in this console")
	}
	catalogs, err := u.Catalog.Catalogs(ctx, false)
	if err != nil {
		return CatalogMatch{}, err
	}
	state, entry, err := resolveCatalogTool(catalogs, request)
	if err != nil {
		return CatalogMatch{}, err
	}
	match := CatalogMatch{Catalog: state.Name, Tool: entry}
	if installed, err := u.Forge.List(ctx, workspace); err == nil {
		if existing, found := findForged(installed, entry.Name); found && existing.Origin != nil && existing.Origin.Catalog == state.Name {
			match.InstalledVersion = existing.Origin.Version
		}
	}
	return match, nil
}

// Stage resolves a request, fetches the tool verified against its signed
// index, checks it as a forged tool and against the workspace's tools,
// and returns what the card shows; nothing is written.
func (u CatalogUseCase) Stage(ctx context.Context, workspace string, request InstallRequest) (StagedTool, error) {
	if !u.Enabled() {
		return StagedTool{}, errors.New("tool catalogs are not enabled in this console")
	}
	if !forgedToolName.MatchString(request.Name) {
		return StagedTool{}, errors.New("name must be 2-48 lowercase letters, digits or underscores, starting with a letter")
	}
	if request.Version != "" && !catalogVersion.MatchString(request.Version) {
		return StagedTool{}, errors.New("version must be major.minor.patch, such as 1.0.0")
	}
	if request.Catalog != "" && !catalogName.MatchString(request.Catalog) {
		return StagedTool{}, errors.New("catalog must be a configured catalog's name")
	}
	catalogs, err := u.Catalog.Cached(ctx)
	if err != nil {
		return StagedTool{}, err
	}
	state, entry, err := resolveCatalogTool(catalogs, request)
	if err != nil {
		return StagedTool{}, err
	}
	installed, err := u.Forge.List(ctx, workspace)
	if err != nil {
		return StagedTool{}, err
	}
	// The collision rule of decision D2: a forged tool of the same name
	// is the person's to remove first; a tool from another catalog too.
	if existing, found := findForged(installed, entry.Name); found {
		switch {
		case existing.Origin == nil:
			return StagedTool{}, fmt.Errorf("a tool named %s was forged in this workspace; remove it with axlr_remove_tool or /tools remove before installing the catalog's", entry.Name)
		case existing.Origin.Catalog != state.Name:
			return StagedTool{}, fmt.Errorf("%s is installed from catalog %s; remove it before installing catalog %s's", entry.Name, existing.Origin.Catalog, state.Name)
		}
	}
	staged, err := u.Catalog.Stage(ctx, state.Name, entry.Name, entry.Version)
	if err != nil {
		return StagedTool{}, err
	}
	if staged.Entry.Name != entry.Name || staged.Entry.Version != entry.Version || staged.Tool.Name != entry.Name {
		return StagedTool{}, fmt.Errorf("catalog %s staged %s %s for %s %s", state.Name, staged.Tool.Name, staged.Entry.Version, entry.Name, entry.Version)
	}
	if err := verifyStagedDigests(staged); err != nil {
		return StagedTool{}, err
	}
	// A manifest names its files by their path in the tool's directory
	// (args: ["seat_allocation.py"]); the console runs them from the
	// workspace root, so an arg that is one of the files becomes its
	// workspace path, as the forge rule requires.
	own := ForgedToolsDir + "/" + entry.Name + "/"
	named := map[string]bool{}
	for _, file := range staged.Files {
		named[path.Clean(file.Path)] = true
	}
	args := make([]string, 0, len(staged.Tool.Args))
	for _, arg := range staged.Tool.Args {
		if named[path.Clean(arg)] {
			arg = own + path.Clean(arg)
		}
		args = append(args, arg)
	}
	files, err := validateForge(staged.Tool.Name, staged.Tool.Description, staged.Tool.InputSchema, staged.Tool.Program, args, staged.Files)
	if err != nil {
		return StagedTool{}, fmt.Errorf("catalog %s's %s %s is not a valid tool: %v", state.Name, entry.Name, entry.Version, err)
	}
	staged.Catalog, staged.Files = state.Name, files
	staged.Tool = ForgedTool{Name: staged.Tool.Name, Description: strings.TrimSpace(staged.Tool.Description), InputSchema: staged.Tool.InputSchema, Program: staged.Tool.Program, Args: args, Origin: &ToolOrigin{Catalog: state.Name, Version: entry.Version, IndexGeneratedAt: staged.IndexGeneratedAt}}
	if entry.Network {
		staged.Warnings = append(staged.Warnings, "this tool reaches the network; under the exec sandbox without network it may not work")
	}
	return staged, nil
}

// Install writes a staged tool into the workspace and registers it with
// its origin, after checking its digests once more; replaced reports a
// previous version. session is the installing session's ID.
func (u CatalogUseCase) Install(ctx context.Context, workspace string, staged StagedTool, session string) (ForgedTool, bool, error) {
	if !u.Enabled() {
		return ForgedTool{}, false, errors.New("tool catalogs are not enabled in this console")
	}
	if staged.Tool.Origin == nil || staged.Tool.Origin.Catalog != staged.Catalog || staged.Tool.Origin.Version != staged.Entry.Version {
		return ForgedTool{}, false, errors.New("the staged tool carries no origin; stage it again")
	}
	if err := verifyStagedDigests(staged); err != nil {
		return ForgedTool{}, false, err
	}
	tool := staged.Tool
	tool.Session = session
	origin := *staged.Tool.Origin
	tool.Origin = &origin
	return u.Forge.Forge(ctx, workspace, tool, staged.Files)
}

// verifyStagedDigests checks every file against the digest the signed
// index carries, and that the files are exactly those the index names.
func verifyStagedDigests(staged StagedTool) error {
	seen := map[string]bool{}
	for _, file := range staged.Files {
		name := path.Clean(file.Path)
		want, listed := staged.Entry.Files[name]
		if !listed {
			return fmt.Errorf("file %s of %s %s is not in the catalog's index; the install is refused", name, staged.Entry.Name, staged.Entry.Version)
		}
		sum := sha256.Sum256([]byte(file.Content))
		if hex.EncodeToString(sum[:]) != strings.ToLower(want) {
			return fmt.Errorf("file %s of %s %s does not match the digest the signed index carries; the install is refused", name, staged.Entry.Name, staged.Entry.Version)
		}
		seen[name] = true
	}
	for name := range staged.Entry.Files {
		if !seen[path.Clean(name)] {
			return fmt.Errorf("file %s of %s %s named by the index was not fetched; the install is refused", name, staged.Entry.Name, staged.Entry.Version)
		}
	}
	if len(staged.Files) == 0 {
		return fmt.Errorf("%s %s has no files; the install is refused", staged.Entry.Name, staged.Entry.Version)
	}
	return nil
}

// resolveCatalogTool picks the catalog and the entry a request names:
// the catalog given, else the only one that has the tool; the version
// given, else the newest.
func resolveCatalogTool(catalogs []CatalogState, request InstallRequest) (CatalogState, CatalogEntry, error) {
	var candidates []CatalogState
	for _, state := range catalogs {
		if request.Catalog != "" && state.Name != request.Catalog {
			continue
		}
		if request.Catalog != "" && state.Index == nil {
			if state.Warning != "" {
				return CatalogState{}, CatalogEntry{}, fmt.Errorf("catalog %s cannot be read: %s", state.Name, state.Warning)
			}
			return CatalogState{}, CatalogEntry{}, fmt.Errorf("catalog %s has not been read yet", state.Name)
		}
		if state.Index == nil {
			continue
		}
		if _, found := latestVersion(state.Index.Tools, request.Name); found {
			candidates = append(candidates, state)
		}
	}
	if request.Catalog != "" && len(candidates) == 0 {
		known := false
		for _, state := range catalogs {
			known = known || state.Name == request.Catalog
		}
		if !known {
			return CatalogState{}, CatalogEntry{}, fmt.Errorf("no catalog is named %s; settings list the catalogs", request.Catalog)
		}
		return CatalogState{}, CatalogEntry{}, fmt.Errorf("catalog %s has no tool named %s", request.Catalog, request.Name)
	}
	switch len(candidates) {
	case 0:
		return CatalogState{}, CatalogEntry{}, fmt.Errorf("no catalog has a tool named %s; axlr_catalog and /tools search find tools", request.Name)
	case 1:
	default:
		names := make([]string, 0, len(candidates))
		for _, state := range candidates {
			names = append(names, state.Name)
		}
		return CatalogState{}, CatalogEntry{}, fmt.Errorf("%s is in catalogs %s; name the catalog", request.Name, strings.Join(names, ", "))
	}
	state := candidates[0]
	if request.Version == "" {
		entry, _ := latestVersion(state.Index.Tools, request.Name)
		return state, entry, nil
	}
	var versions []string
	for _, tool := range state.Index.Tools {
		if tool.Name != request.Name {
			continue
		}
		if tool.Version == request.Version {
			return state, tool, nil
		}
		versions = append(versions, tool.Version)
	}
	return CatalogState{}, CatalogEntry{}, fmt.Errorf("catalog %s has no %s %s; it has %s", state.Name, request.Name, request.Version, strings.Join(versions, ", "))
}

// latestVersion is the newest entry named name.
func latestVersion(tools []CatalogEntry, name string) (CatalogEntry, bool) {
	var latest CatalogEntry
	found := false
	for _, tool := range tools {
		if tool.Name != name {
			continue
		}
		if !found || newerVersion(tool.Version, latest.Version) {
			latest, found = tool, true
		}
	}
	return latest, found
}

// NewestOfEach keeps the newest version of each name, in name order.
func NewestOfEach(tools []CatalogEntry) []CatalogEntry {
	byName := map[string]CatalogEntry{}
	for _, tool := range tools {
		if latest, seen := byName[tool.Name]; !seen || newerVersion(tool.Version, latest.Version) {
			byName[tool.Name] = tool
		}
	}
	out := make([]CatalogEntry, 0, len(byName))
	for _, tool := range byName {
		out = append(out, tool)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// newerVersion reports a > b for major.minor.patch versions; a version
// that does not parse is older than one that does.
func newerVersion(a, b string) bool {
	ap, aok := parseVersion(a)
	bp, bok := parseVersion(b)
	if aok != bok {
		return aok
	}
	for i := range ap {
		if ap[i] != bp[i] {
			return ap[i] > bp[i]
		}
	}
	return false
}

func parseVersion(v string) ([3]int, bool) {
	var parts [3]int
	fields := strings.Split(v, ".")
	if len(fields) != 3 {
		return parts, false
	}
	for i, field := range fields {
		n, err := strconv.Atoi(field)
		if err != nil || n < 0 {
			return parts, false
		}
		parts[i] = n
	}
	return parts, true
}
