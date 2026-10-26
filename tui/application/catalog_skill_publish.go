package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// Publishing a skill sends the workspace's .axlr/skills/<name>/ to a
// catalog's registry: SKILL.md (its front matter gives the description and
// the tools it expects) and up to seven more text files make the bundle,
// catalog.json beside them the catalog block, and tests.json, optional, the
// harness the registry checks. The bundle is signed with the same client
// key and uploaded to the same POST /v1/versions as a tool's.

// SkillSourceDir is where a workspace keeps the skills it publishes.
const SkillSourceDir = ".axlr/skills"

// SkillSourcePort reads a skill's source directory in the workspace.
type SkillSourcePort interface {
	// ReadSkillSource returns the regular files of
	// <workspace>/.axlr/skills/<name>/ by name; ErrNoSkillSource when the
	// directory is not there.
	ReadSkillSource(ctx context.Context, workspace, name string) (map[string][]byte, error)
}

// ErrNoSkillSource is ReadSkillSource's answer for a missing directory.
var ErrNoSkillSource = errors.New("no such skill in the workspace's " + SkillSourceDir)

// SkillDraft is a skill read and checked for publishing.
type SkillDraft struct {
	Catalog, API      string
	Name, Description string
	Tools             []string
	Files             map[string][]byte
	Tests             []byte
	Metadata          SkillMetadata
}

// SkillMetadata is a skill manifest's catalog block, as catalog.json keeps
// it: a tool's without what a program needs.
type SkillMetadata struct {
	Version  string   `json:"version"`
	Category string   `json:"category"`
	Tags     []string `json:"tags"`
	License  string   `json:"license"`
	Source   string   `json:"source,omitempty"`
}

// skillManifest is a skill's manifest.json as the registry parses it, with
// the keys in this order.
type skillManifest struct {
	Kind        string        `json:"kind"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Files       []string      `json:"files"`
	Tools       []string      `json:"tools"`
	Catalog     SkillMetadata `json:"catalog"`
}

var skillToolReference = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}(/[A-Za-z0-9_.-]{1,64})?$`)

// Publishable reports whether the console can publish skills.
func (u SkillCatalogUseCase) Publishable() bool {
	return u.Catalog != nil && u.Keys != nil && u.Registry != nil && u.Sources != nil
}

// PrepareSkill reads a skill of the workspace for publishing and checks it
// as the registry will.
func (u SkillCatalogUseCase) PrepareSkill(ctx context.Context, workspace string, request PublishRequest) (SkillDraft, error) {
	if !u.Publishable() {
		return SkillDraft{}, errors.New("publishing skills to a registry is not enabled in this console")
	}
	if !forgedToolName.MatchString(request.Name) {
		return SkillDraft{}, errors.New("name must be 2-48 lowercase letters, digits or underscores, starting with a letter")
	}
	if request.Version != "" && !catalogVersion.MatchString(request.Version) {
		return SkillDraft{}, errors.New("version must be major.minor.patch, such as 1.0.0")
	}
	catalogs, err := u.Catalog.Cached(ctx)
	if err != nil {
		return SkillDraft{}, err
	}
	state, err := resolveRegistry(catalogs, request.Catalog)
	if err != nil {
		return SkillDraft{}, err
	}
	files, err := u.Sources.ReadSkillSource(ctx, workspace, request.Name)
	if err != nil {
		return SkillDraft{}, err
	}
	draft := SkillDraft{Catalog: state.Name, API: state.APIURL, Name: request.Name, Files: map[string][]byte{}}
	meta, has := files[PublishCatalogFile]
	if !has {
		return SkillDraft{}, fmt.Errorf("%s/%s/%s is missing: it holds version, category, license and tags", SkillSourceDir, request.Name, PublishCatalogFile)
	}
	if err := strictJSON(meta, &draft.Metadata); err != nil {
		return SkillDraft{}, fmt.Errorf("%s/%s/%s is not a skill's catalog block (version, category, tags, license, source): %v", SkillSourceDir, request.Name, PublishCatalogFile, err)
	}
	if request.Version != "" {
		draft.Metadata.Version = request.Version
	}
	if err := checkSkillMetadata(draft.Metadata); err != nil {
		return SkillDraft{}, err
	}
	if tests, has := files[PublishTestsFile]; has {
		if err := checkSkillTests(tests); err != nil {
			return SkillDraft{}, fmt.Errorf("%s/%s/%s: %v", SkillSourceDir, request.Name, PublishTestsFile, err)
		}
		draft.Tests = tests
	}
	for name, content := range files {
		if name == PublishCatalogFile || name == PublishTestsFile {
			continue
		}
		if !publishFileName.MatchString(name) || strings.HasPrefix(name, ".") {
			return SkillDraft{}, fmt.Errorf("file %s of the skill is not a flat name of letters, digits, '.', '_' or '-'", name)
		}
		if len(content) == 0 || len(content) > MaxSkillFileBytes || !utf8.Valid(content) || strings.ContainsRune(string(content), 0) {
			return SkillDraft{}, fmt.Errorf("file %s of the skill is not UTF-8 text of 1 byte to 256 KiB", name)
		}
		draft.Files[name] = content
	}
	if len(draft.Files) > 8 {
		return SkillDraft{}, fmt.Errorf("a skill has at most eight files; %s has %d beside %s and %s", request.Name, len(draft.Files), PublishCatalogFile, PublishTestsFile)
	}
	main, has := draft.Files[SkillMainFile]
	if !has {
		return SkillDraft{}, fmt.Errorf("%s/%s has no %s", SkillSourceDir, request.Name, SkillMainFile)
	}
	front := skillFrontMatter(string(main))
	if name, has := front["name"]; has && name != request.Name {
		return SkillDraft{}, fmt.Errorf("%s names the skill %q; the directory and the catalog name it %s", SkillMainFile, name, request.Name)
	}
	draft.Description = strings.TrimSpace(front["description"])
	if draft.Description == "" || len(draft.Description) > 1024 {
		return SkillDraft{}, fmt.Errorf("%s's front matter needs a description of 1 to 1024 characters: when the model should open the skill", SkillMainFile)
	}
	draft.Tools = skillFrontList(front["tools"])
	if len(draft.Tools) > 32 {
		return SkillDraft{}, errors.New("a skill names at most 32 tools")
	}
	seen := map[string]bool{}
	for _, tool := range draft.Tools {
		if !skillToolReference.MatchString(tool) || seen[tool] {
			return SkillDraft{}, fmt.Errorf("tool %q of the skill's front matter is not a tool name (local_read, a catalog tool, server/tool) or is listed twice", tool)
		}
		seen[tool] = true
	}
	return draft, nil
}

// BundleSkill builds the signed upload of a draft: the manifest, the
// bundle digest and its signature with the client key, created when there
// is none.
func (u SkillCatalogUseCase) BundleSkill(ctx context.Context, draft SkillDraft) (PublishBundle, error) {
	if !u.Publishable() {
		return PublishBundle{}, errors.New("publishing skills to a registry is not enabled in this console")
	}
	names := make([]string, 0, len(draft.Files))
	for name := range draft.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	tools := draft.Tools
	if tools == nil {
		tools = []string{}
	}
	meta := draft.Metadata
	if meta.Tags == nil {
		meta.Tags = []string{}
	}
	encoded, err := json.MarshalIndent(skillManifest{Kind: "skill", Name: draft.Name, Description: draft.Description, Files: names, Tools: tools, Catalog: meta}, "", "  ")
	if err != nil {
		return PublishBundle{}, err
	}
	encoded = append(encoded, '\n')
	key, created, err := CatalogUseCase{Keys: u.Keys}.clientKey(ctx)
	if err != nil {
		return PublishBundle{}, err
	}
	digest := BundleDigest(encoded, draft.Files)
	return PublishBundle{
		Upload:     UploadRequest{Manifest: encoded, Files: draft.Files, Tests: draft.Tests, KeyID: key.ID(), Signature: key.Sign(BundleSignatureMessage(digest))},
		Digest:     digest,
		KeyID:      key.ID(),
		KeyCreated: created,
	}, nil
}

// SubmitSkill uploads a bundle to the draft's registry.
func (u SkillCatalogUseCase) SubmitSkill(ctx context.Context, draft SkillDraft, bundle PublishBundle) (VersionReport, error) {
	key, err := u.publishKey(ctx)
	if err != nil {
		return VersionReport{}, err
	}
	if key.ID() != bundle.KeyID {
		return VersionReport{}, errors.New("the client key changed since the bundle was signed; publish again")
	}
	return u.Registry.Submit(ctx, draft.API, key, bundle.Upload)
}

// PollSkill reads the draft's version from the registry.
func (u SkillCatalogUseCase) PollSkill(ctx context.Context, draft SkillDraft) (VersionReport, error) {
	key, err := u.publishKey(ctx)
	if err != nil {
		return VersionReport{}, err
	}
	return u.Registry.Version(ctx, draft.API, key, draft.Name, draft.Metadata.Version)
}

func (u SkillCatalogUseCase) publishKey(ctx context.Context) (*ClientKey, error) {
	if !u.Publishable() {
		return nil, errors.New("publishing skills to a registry is not enabled in this console")
	}
	key, _, err := CatalogUseCase{Keys: u.Keys}.clientKey(ctx)
	return key, err
}

func checkSkillMetadata(meta SkillMetadata) error {
	var missing []string
	if !catalogVersion.MatchString(meta.Version) {
		missing = append(missing, "version (major.minor.patch)")
	}
	if !publishCategory.MatchString(meta.Category) {
		missing = append(missing, "category")
	}
	if !publishLicense.MatchString(meta.License) {
		missing = append(missing, "license (an SPDX id)")
	}
	for _, tag := range meta.Tags {
		if !publishTag.MatchString(tag) {
			missing = append(missing, fmt.Sprintf("tag %q (lowercase letters, digits, _ or -)", tag))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("the skill's %s lacks or misspells %s", PublishCatalogFile, strings.Join(missing, ", "))
	}
	return nil
}

// checkSkillTests reads a skill's tests.json as the registry does: a
// non-empty list of model cases with distinct names, a prompt and an
// expectation of tools and answer fragments.
func checkSkillTests(data []byte) error {
	var cases []struct {
		Name   string `json:"name"`
		Prompt string `json:"prompt"`
		Expect struct {
			Tools          []string `json:"tools"`
			AnswerContains []string `json:"answer_contains"`
		} `json:"expect"`
	}
	if err := strictJSON(data, &cases); err != nil {
		return fmt.Errorf("not a list of cases {name, prompt, expect: {tools, answer_contains}}: %v", err)
	}
	if len(cases) == 0 {
		return errors.New("no cases")
	}
	seen := map[string]bool{}
	for _, c := range cases {
		if strings.TrimSpace(c.Name) == "" || strings.TrimSpace(c.Prompt) == "" {
			return errors.New("every case has a name and a prompt")
		}
		if seen[c.Name] {
			return fmt.Errorf("two cases are named %q", c.Name)
		}
		seen[c.Name] = true
	}
	return nil
}

func strictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.More() {
		return errors.New("trailing data")
	}
	return nil
}

// skillFrontMatter reads the plain scalars of a SKILL.md front matter, a
// folded or literal block (`key: >-` and its indented lines) as one line,
// and a block list (`- item`, indented or not) as its items joined by
// newlines; text without front matter has none.
func skillFrontMatter(text string) map[string]string {
	fields := map[string]string{}
	lines := strings.Split(strings.ReplaceAll(strings.TrimPrefix(text, "\ufeff"), "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return fields
	}
	current, block := "", false
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			break
		}
		indented := strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
		if block && (indented || trimmed == "") {
			if trimmed != "" {
				fields[current] = strings.TrimSpace(fields[current] + " " + trimmed)
			}
			continue
		}
		block = false
		if item, ok := strings.CutPrefix(trimmed, "- "); ok && current != "" {
			fields[current] += "\n" + unquote(strings.TrimSpace(item))
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok || indented {
			continue
		}
		current, value = strings.TrimSpace(key), strings.TrimSpace(value)
		switch value {
		case ">", ">-", ">+", "|", "|-", "|+":
			fields[current], block = "", true
		default:
			fields[current] = unquote(value)
		}
	}
	return fields
}

// skillFrontList reads a front matter list, inline ([a, b]) or block.
func skillFrontList(value string) []string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")
	var items []string
	for _, item := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '\n' }) {
		if item = unquote(strings.TrimSpace(item)); item != "" {
			items = append(items, item)
		}
	}
	return items
}

func unquote(value string) string {
	if len(value) >= 2 && (value[0] == '"' && value[len(value)-1] == '"' || value[0] == '\'' && value[len(value)-1] == '\'') {
		return value[1 : len(value)-1]
	}
	return value
}
