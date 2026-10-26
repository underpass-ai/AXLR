package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// A catalog skill (docs/plans/2026-10-26-catalog-skills-design.md) is a
// versioned, signed procedure the model reads with axlr_skill: SKILL.md and
// up to seven more text files, the tools it expects, and the provenance of
// the signed index it came from. Installed skills live per user, every
// version side by side, so an execution can pin one while another reads
// the newest.

// CatalogSkillEntry is one skill of an index.
type CatalogSkillEntry struct {
	Name           string            `json:"name"`
	Version        string            `json:"version"`
	Category       string            `json:"category"`
	Tags           []string          `json:"tags"`
	Summary        string            `json:"summary"`
	License        string            `json:"license"`
	Manifest       string            `json:"manifest"`
	ManifestSHA256 string            `json:"manifest_sha256"`
	Files          map[string]string `json:"files"`
	Tools          []string          `json:"tools"`
	UpdatedAt      string            `json:"updated_at,omitempty"`
	Publisher      string            `json:"publisher,omitempty"`
	PublisherKeyID string            `json:"publisher_key_id,omitempty"`
	BundleSHA256   string            `json:"bundle_sha256,omitempty"`
	Signature      string            `json:"signature,omitempty"`
	ReviewedAt     string            `json:"reviewed_at,omitempty"`
	Reviewer       string            `json:"reviewer,omitempty"`
	Warnings       []string          `json:"warnings,omitempty"`
}

// SkillFile is one file of a skill, by its name in the skill's directory.
type SkillFile struct {
	Path    string
	Content []byte
}

// SkillMainFile is the file every skill has and the model opens first.
const SkillMainFile = "SKILL.md"

// MaxSkillFileBytes bounds one file of a catalog skill, as the registry
// does.
const MaxSkillFileBytes = 256 << 10

// SkillStagePort fetches a catalog skill's manifest and files for one
// entry of a cached index, each verified against the digests the signed
// index carries; it writes nothing and never refreshes the index.
type SkillStagePort interface {
	StageSkill(ctx context.Context, catalog, name, version string) (StagedSkill, error)
}

// StagedSkill is a catalog skill fetched and verified, ready for its card
// and, once approved, for the install.
type StagedSkill struct {
	Catalog          string
	Entry            CatalogSkillEntry
	IndexGeneratedAt string
	Description      string
	Tools            []string
	Files            []SkillFile
	// Missing lists the tools the skill expects that this console does not
	// offer; Warnings what else the card says.
	Missing  []string
	Warnings []string
}

// Key names a staged skill: the catalog, the skill and the version.
func (s StagedSkill) Key() string { return s.Catalog + "/" + s.Entry.Name + "@" + s.Entry.Version }

// InstalledSkill is the record of one installed version.
type InstalledSkill struct {
	Catalog          string            `json:"catalog"`
	Name             string            `json:"name"`
	Version          string            `json:"version"`
	Description      string            `json:"description"`
	Tools            []string          `json:"tools"`
	Files            map[string]string `json:"files"`
	ManifestSHA256   string            `json:"manifest_sha256"`
	BundleSHA256     string            `json:"bundle_sha256,omitempty"`
	IndexGeneratedAt string            `json:"index_generated_at"`
	Publisher        string            `json:"publisher,omitempty"`
	Reviewer         string            `json:"reviewer,omitempty"`
	InstalledAt      time.Time         `json:"installed_at"`
}

// Key names an installed skill version.
func (s InstalledSkill) Key() string { return s.Catalog + "/" + s.Name + "@" + s.Version }

// InstalledSkillsPort keeps the installed catalog skills.
type InstalledSkillsPort interface {
	// List returns every installed version, by catalog, name and version.
	List(ctx context.Context) ([]InstalledSkill, error)
	// Install writes one version's files and record; replaced reports the
	// same version installed before.
	Install(ctx context.Context, skill InstalledSkill, files []SkillFile) (bool, error)
	// Remove removes one version, or every version of the name when
	// version is "", and returns what it removed.
	Remove(ctx context.Context, catalog, name, version string) ([]InstalledSkill, error)
	// Read returns one file of an installed version, checked against the
	// digest its record keeps.
	Read(ctx context.Context, catalog, name, version, file string) ([]byte, error)
}

// SkillMatch is one catalog skill found, with the versions installed.
type SkillMatch struct {
	Catalog   string
	Skill     CatalogSkillEntry
	Installed []string
}

// SkillUpdate is an installed skill whose catalog has a newer version.
type SkillUpdate struct {
	Name, Catalog, Installed, Available string
}

// SkillOverview is what /skills shows.
type SkillOverview struct {
	Installed []InstalledSkill
	Catalogs  []CatalogState
	Updates   []SkillUpdate
}

// SkillCatalogUseCase installs, lists and removes catalog skills, and
// publishes the workspace's own, for the person (/skills) and for programs
// (axlr-tui skills).
type SkillCatalogUseCase struct {
	Catalog   CatalogPort
	Stager    SkillStagePort
	Installed InstalledSkillsPort
	// Offered names the tools this console offers, for the card's list of
	// missing ones; nil marks none missing.
	Offered func(ctx context.Context) []string
	// Keys, Registry and Sources serve publishing; nil leaves it off.
	Keys     ClientKeyPort
	Registry RegistryPort
	Sources  SkillSourcePort
}

// Enabled reports whether the catalogs and the install are set.
func (u SkillCatalogUseCase) Enabled() bool {
	return u.Catalog != nil && u.Stager != nil && u.Installed != nil
}

var errSkillsOff = errors.New("catalog skills are not enabled in this console")

// Overview reads the installed skills and the catalogs.
func (u SkillCatalogUseCase) Overview(ctx context.Context, refresh bool) (SkillOverview, error) {
	if !u.Enabled() {
		return SkillOverview{}, errSkillsOff
	}
	installed, err := u.Installed.List(ctx)
	if err != nil {
		return SkillOverview{}, err
	}
	catalogs, err := u.Catalog.Catalogs(ctx, refresh)
	if err != nil {
		return SkillOverview{}, err
	}
	overview := SkillOverview{Installed: installed, Catalogs: catalogs}
	for _, skill := range NewestInstalled(installed) {
		for _, state := range catalogs {
			if state.Name != skill.Catalog || state.Index == nil {
				continue
			}
			if latest, found := latestSkill(state.Index.Skills, skill.Name); found && newerVersion(latest.Version, skill.Version) {
				overview.Updates = append(overview.Updates, SkillUpdate{Name: skill.Name, Catalog: state.Name, Installed: skill.Version, Available: latest.Version})
			}
		}
	}
	return overview, nil
}

// Search finds the catalog skills whose name, tags, category or summary
// contain every term, every skill when there is none, the newest version
// of each name.
func (u SkillCatalogUseCase) Search(ctx context.Context, terms []string) ([]SkillMatch, []CatalogState, error) {
	if !u.Enabled() {
		return nil, nil, errSkillsOff
	}
	installed, err := u.Installed.List(ctx)
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
	var matches []SkillMatch
	for _, state := range catalogs {
		if state.Index == nil {
			continue
		}
		for _, skill := range newestSkills(state.Index.Skills) {
			text := strings.ToLower(skill.Name + " " + skill.Category + " " + strings.Join(skill.Tags, " ") + " " + skill.Summary)
			if termsMatched(text, lowered) != len(lowered) {
				continue
			}
			match := SkillMatch{Catalog: state.Name, Skill: skill}
			for _, have := range installed {
				if have.Catalog == state.Name && have.Name == skill.Name {
					match.Installed = append(match.Installed, have.Version)
				}
			}
			matches = append(matches, match)
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Skill.Name != matches[j].Skill.Name {
			return matches[i].Skill.Name < matches[j].Skill.Name
		}
		return matches[i].Catalog < matches[j].Catalog
	})
	return matches, catalogs, nil
}

// Stage resolves a request against the cached indexes, fetches the skill
// verified against its signed index, and returns what the card shows;
// nothing is written.
func (u SkillCatalogUseCase) Stage(ctx context.Context, request InstallRequest) (StagedSkill, error) {
	if !u.Enabled() {
		return StagedSkill{}, errSkillsOff
	}
	if err := checkInstallRequest(request); err != nil {
		return StagedSkill{}, err
	}
	catalogs, err := u.Catalog.Cached(ctx)
	if err != nil {
		return StagedSkill{}, err
	}
	state, entry, err := resolveCatalogSkill(catalogs, request)
	if err != nil {
		return StagedSkill{}, err
	}
	staged, err := u.Stager.StageSkill(ctx, state.Name, entry.Name, entry.Version)
	if err != nil {
		return StagedSkill{}, err
	}
	if staged.Entry.Name != entry.Name || staged.Entry.Version != entry.Version {
		return StagedSkill{}, fmt.Errorf("catalog %s staged %s %s for %s %s", state.Name, staged.Entry.Name, staged.Entry.Version, entry.Name, entry.Version)
	}
	staged.Catalog = state.Name
	if err := verifyStagedSkill(staged); err != nil {
		return StagedSkill{}, err
	}
	staged.Missing = nil
	if u.Offered != nil {
		offered := map[string]bool{}
		for _, name := range u.Offered(ctx) {
			offered[name] = true
		}
		for _, tool := range staged.Tools {
			if !offered[tool] {
				staged.Missing = append(staged.Missing, tool)
			}
		}
	}
	if installed, err := u.Installed.List(ctx); err == nil {
		for _, have := range installed {
			if have.Catalog == staged.Catalog && have.Name == entry.Name && have.Version == entry.Version {
				staged.Warnings = append(staged.Warnings, "this version is installed already; installing it again rewrites its files")
			}
		}
	}
	return staged, nil
}

// Install writes a staged skill beside the versions already installed,
// after checking its digests once more.
func (u SkillCatalogUseCase) Install(ctx context.Context, staged StagedSkill, now time.Time) (InstalledSkill, bool, error) {
	if !u.Enabled() {
		return InstalledSkill{}, false, errSkillsOff
	}
	if staged.Catalog == "" {
		return InstalledSkill{}, false, errors.New("the staged skill carries no catalog; stage it again")
	}
	if err := verifyStagedSkill(staged); err != nil {
		return InstalledSkill{}, false, err
	}
	record := InstalledSkill{
		Catalog: staged.Catalog, Name: staged.Entry.Name, Version: staged.Entry.Version, Description: staged.Description,
		Tools: append([]string{}, staged.Tools...), Files: map[string]string{}, ManifestSHA256: strings.ToLower(staged.Entry.ManifestSHA256),
		BundleSHA256: strings.ToLower(staged.Entry.BundleSHA256), IndexGeneratedAt: staged.IndexGeneratedAt,
		Publisher: staged.Entry.Publisher, Reviewer: staged.Entry.Reviewer, InstalledAt: now.UTC(),
	}
	for _, file := range staged.Files {
		record.Files[file.Path] = sha256Hex(file.Content)
	}
	if record.BundleSHA256 == "" {
		// A static catalog's index carries no bundle digest: the console
		// computes it the way the registry does, so a pin still names the
		// bundle it read.
		files := map[string][]byte{}
		for _, file := range staged.Files {
			files[file.Path] = file.Content
		}
		record.BundleSHA256 = skillBundleDigest(record.ManifestSHA256, files)
	}
	replaced, err := u.Installed.Install(ctx, record, staged.Files)
	if err != nil {
		return InstalledSkill{}, false, err
	}
	return record, replaced, nil
}

// Remove removes one installed version, or every version of a name; a
// name installed from two catalogs needs the catalog.
func (u SkillCatalogUseCase) Remove(ctx context.Context, request InstallRequest) ([]InstalledSkill, error) {
	if !u.Enabled() {
		return nil, errSkillsOff
	}
	if err := checkInstallRequest(request); err != nil {
		return nil, err
	}
	installed, err := u.Installed.List(ctx)
	if err != nil {
		return nil, err
	}
	catalogs := map[string]bool{}
	for _, have := range installed {
		if have.Name == request.Name && (request.Catalog == "" || have.Catalog == request.Catalog) && (request.Version == "" || have.Version == request.Version) {
			catalogs[have.Catalog] = true
		}
	}
	switch len(catalogs) {
	case 0:
		if request.Version != "" {
			return nil, fmt.Errorf("skill %s %s is not installed", request.Name, request.Version)
		}
		return nil, fmt.Errorf("skill %s is not installed", request.Name)
	case 1:
	default:
		names := make([]string, 0, len(catalogs))
		for name := range catalogs {
			names = append(names, name)
		}
		sort.Strings(names)
		return nil, fmt.Errorf("%s is installed from catalogs %s; name the catalog", request.Name, strings.Join(names, ", "))
	}
	var catalog string
	for name := range catalogs {
		catalog = name
	}
	return u.Installed.Remove(ctx, catalog, request.Name, request.Version)
}

// checkInstallRequest validates a name, version and catalog as typed.
func checkInstallRequest(request InstallRequest) error {
	if !forgedToolName.MatchString(request.Name) {
		return errors.New("name must be 2-48 lowercase letters, digits or underscores, starting with a letter")
	}
	if request.Version != "" && !catalogVersion.MatchString(request.Version) {
		return errors.New("version must be major.minor.patch, such as 1.0.0")
	}
	if request.Catalog != "" && !catalogName.MatchString(request.Catalog) {
		return errors.New("catalog must be a configured catalog's name")
	}
	return nil
}

// verifyStagedSkill checks every file against the digest the signed index
// carries, that the files are exactly those it names, and that SKILL.md is
// among them, UTF-8 text like every file.
func verifyStagedSkill(staged StagedSkill) error {
	label := staged.Entry.Name + " " + staged.Entry.Version
	seen := map[string]bool{}
	for _, file := range staged.Files {
		name := path.Clean(file.Path)
		if name != file.Path || strings.Contains(name, "/") {
			return fmt.Errorf("file %q of skill %s is not a flat name; the install is refused", file.Path, label)
		}
		want, listed := staged.Entry.Files[name]
		if !listed {
			return fmt.Errorf("file %s of skill %s is not in the catalog's index; the install is refused", name, label)
		}
		if sha256Hex(file.Content) != strings.ToLower(want) {
			return fmt.Errorf("file %s of skill %s does not match the digest the signed index carries; the install is refused", name, label)
		}
		if len(file.Content) > MaxSkillFileBytes || !utf8.Valid(file.Content) || strings.ContainsRune(string(file.Content), 0) {
			return fmt.Errorf("file %s of skill %s is not UTF-8 text of at most 256 KiB; the install is refused", name, label)
		}
		seen[name] = true
	}
	for name := range staged.Entry.Files {
		if !seen[name] {
			return fmt.Errorf("file %s of skill %s named by the index was not fetched; the install is refused", name, label)
		}
	}
	if !seen[SkillMainFile] {
		return fmt.Errorf("skill %s has no %s; the install is refused", label, SkillMainFile)
	}
	return nil
}

// skillBundleDigest is the registry's bundle digest from the manifest's
// digest and the files.
func skillBundleDigest(manifestSHA256 string, files map[string][]byte) string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var listing strings.Builder
	listing.WriteString("manifest " + manifestSHA256 + "\n")
	for _, name := range names {
		listing.WriteString("file " + name + " " + sha256Hex(files[name]) + "\n")
	}
	sum := sha256.Sum256([]byte(listing.String()))
	return hex.EncodeToString(sum[:])
}

// resolveCatalogSkill picks the catalog and the entry a request names:
// the catalog given, else the only one that has the skill; the version
// given, else the newest.
func resolveCatalogSkill(catalogs []CatalogState, request InstallRequest) (CatalogState, CatalogSkillEntry, error) {
	var candidates []CatalogState
	known := false
	for _, state := range catalogs {
		if request.Catalog != "" && state.Name != request.Catalog {
			continue
		}
		known = true
		if request.Catalog != "" && state.Index == nil {
			if state.Warning != "" {
				return CatalogState{}, CatalogSkillEntry{}, fmt.Errorf("catalog %s cannot be read: %s", state.Name, state.Warning)
			}
			return CatalogState{}, CatalogSkillEntry{}, fmt.Errorf("catalog %s has not been read yet", state.Name)
		}
		if state.Index == nil {
			continue
		}
		if _, found := latestSkill(state.Index.Skills, request.Name); found {
			candidates = append(candidates, state)
		}
	}
	if request.Catalog != "" && !known {
		return CatalogState{}, CatalogSkillEntry{}, fmt.Errorf("no catalog is named %s; settings list the catalogs", request.Catalog)
	}
	switch len(candidates) {
	case 0:
		if request.Catalog != "" {
			return CatalogState{}, CatalogSkillEntry{}, fmt.Errorf("catalog %s has no skill named %s", request.Catalog, request.Name)
		}
		return CatalogState{}, CatalogSkillEntry{}, fmt.Errorf("no catalog has a skill named %s; /skills search finds skills", request.Name)
	case 1:
	default:
		names := make([]string, 0, len(candidates))
		for _, state := range candidates {
			names = append(names, state.Name)
		}
		return CatalogState{}, CatalogSkillEntry{}, fmt.Errorf("%s is in catalogs %s; name the catalog", request.Name, strings.Join(names, ", "))
	}
	state := candidates[0]
	if request.Version == "" {
		entry, _ := latestSkill(state.Index.Skills, request.Name)
		return state, entry, nil
	}
	var versions []string
	for _, skill := range state.Index.Skills {
		if skill.Name != request.Name {
			continue
		}
		if skill.Version == request.Version {
			return state, skill, nil
		}
		versions = append(versions, skill.Version)
	}
	return CatalogState{}, CatalogSkillEntry{}, fmt.Errorf("catalog %s has no %s %s; it has %s", state.Name, request.Name, request.Version, strings.Join(versions, ", "))
}

// latestSkill is the newest entry named name.
func latestSkill(skills []CatalogSkillEntry, name string) (CatalogSkillEntry, bool) {
	var latest CatalogSkillEntry
	found := false
	for _, skill := range skills {
		if skill.Name == name && (!found || newerVersion(skill.Version, latest.Version)) {
			latest, found = skill, true
		}
	}
	return latest, found
}

// newestSkills keeps the newest version of each name, in name order.
func newestSkills(skills []CatalogSkillEntry) []CatalogSkillEntry {
	byName := map[string]CatalogSkillEntry{}
	for _, skill := range skills {
		if latest, seen := byName[skill.Name]; !seen || newerVersion(skill.Version, latest.Version) {
			byName[skill.Name] = skill
		}
	}
	out := make([]CatalogSkillEntry, 0, len(byName))
	for _, skill := range byName {
		out = append(out, skill)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// NewestInstalled keeps the newest installed version of each catalog's
// skill, by catalog then name.
func NewestInstalled(installed []InstalledSkill) []InstalledSkill {
	byKey := map[string]InstalledSkill{}
	for _, skill := range installed {
		key := skill.Catalog + "/" + skill.Name
		if latest, seen := byKey[key]; !seen || newerVersion(skill.Version, latest.Version) {
			byKey[key] = skill
		}
	}
	out := make([]InstalledSkill, 0, len(byKey))
	for _, skill := range byKey {
		out = append(out, skill)
	}
	sortInstalled(out)
	return out
}

// sortInstalled orders by catalog, name and version, oldest first.
func sortInstalled(skills []InstalledSkill) {
	sort.Slice(skills, func(i, j int) bool {
		a, b := skills[i], skills[j]
		if a.Catalog != b.Catalog {
			return a.Catalog < b.Catalog
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return newerVersion(b.Version, a.Version)
	})
}
