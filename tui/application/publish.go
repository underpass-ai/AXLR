package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Publishing (axlr-registry docs/client-identity.md §5) sends a tool
// forged in this workspace to a catalog's registry: the console builds the
// bundle from the registered files plus the tool's tests.json and a
// manifest catalog block, signs the bundle and the request with the client
// key, uploads it and polls the version until the policy scan and the
// sandbox tests settle. The catalog block's fields live in
// ForgedToolsDir/<name>/catalog.json, which the forge card ignores and a
// forge again removes with the directory.

// Polling cadence of /tools publish.
const (
	PublishPollInterval = 5 * time.Second
	PublishPollDeadline = 4 * time.Minute
	// PublishCatalogFile and PublishTestsFile are the two files beside a
	// tool's own that publishing reads; neither is part of the bundle.
	PublishCatalogFile = "catalog.json"
	PublishTestsFile   = "tests.json"
)

// ToolFilesPort reads and writes files in a forged tool's directory,
// without registering anything.
type ToolFilesPort interface {
	// ReadToolFile returns ErrNoToolFile for a file that is not there.
	ReadToolFile(ctx context.Context, workspace, tool, file string) ([]byte, error)
	WriteToolFile(ctx context.Context, workspace, tool, file string, data []byte) error
}

// ErrNoToolFile is ReadToolFile's answer for a missing file.
var ErrNoToolFile = errors.New("no such file in the tool's directory")

// PublishMetadata is the manifest's catalog block, in the registry's shape
// (crates/domain/src/catalog_block.rs); it is what catalog.json keeps.
type PublishMetadata struct {
	Version  string          `json:"version"`
	Category string          `json:"category"`
	Tags     []string        `json:"tags"`
	License  string          `json:"license"`
	Requires CatalogRequires `json:"requires"`
	Network  bool            `json:"network"`
	Example  json.RawMessage `json:"example,omitempty"`
	Source   string          `json:"source,omitempty"`
}

// PublishRequest names what to publish: a tool forged here, the version
// (empty keeps catalog.json's or asks) and the catalog (empty is the only
// one with an api_url).
type PublishRequest struct {
	Name, Version, Catalog string
}

// PublishDraft is a tool read and checked for publishing, before the
// missing fields are asked and the bundle is built.
type PublishDraft struct {
	Catalog  string
	API      string
	Tool     ForgedTool
	Files    map[string][]byte
	Tests    []byte
	Metadata PublishMetadata
	// Missing lists the catalog fields still to ask, in the order to ask
	// them: version, category, license, tags.
	Missing  []string
	Warnings []string
}

// PublishBundle is the signed upload: the manifest as sent, its bundle
// digest and the signature's key.
type PublishBundle struct {
	Upload     UploadRequest
	Digest     string
	KeyID      string
	KeyCreated bool
}

// publishManifest is manifest.json as the registry parses it (no unknown
// field), with the keys in this order.
type publishManifest struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
	Program     string          `json:"program"`
	Args        []string        `json:"args"`
	Files       []string        `json:"files"`
	Catalog     PublishMetadata `json:"catalog"`
}

var (
	publishCategory = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,47}$`)
	publishLicense  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9.+-]{0,63}$`)
	publishTag      = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)
	publishFileName = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)
	publisherID     = regexp.MustCompile(`^[a-z][a-z0-9_-]{1,31}$`)
	publishField    = []string{"version", "category", "license", "tags"}
)

// Publishable reports whether the console can publish: the catalogs, the
// forge, the key and the registry ports are set.
func (u CatalogUseCase) Publishable() bool {
	return u.Enabled() && u.Keys != nil && u.Registry != nil && u.Files != nil
}

// Prepare reads a forged tool for publishing: the catalog's api_url, the
// registered files verified against their digests, tests.json and
// catalog.json, and which catalog fields are still missing.
func (u CatalogUseCase) Prepare(ctx context.Context, workspace string, request PublishRequest) (PublishDraft, error) {
	if !u.Publishable() {
		return PublishDraft{}, errors.New("publishing to a registry is not enabled in this console")
	}
	if !forgedToolName.MatchString(request.Name) {
		return PublishDraft{}, errors.New("name must be 2-48 lowercase letters, digits or underscores, starting with a letter")
	}
	if request.Version != "" && !catalogVersion.MatchString(request.Version) {
		return PublishDraft{}, errors.New("version must be major.minor.patch, such as 1.0.0")
	}
	catalogs, err := u.Catalog.Cached(ctx)
	if err != nil {
		return PublishDraft{}, err
	}
	state, err := resolveRegistry(catalogs, request.Catalog)
	if err != nil {
		return PublishDraft{}, err
	}
	installed, err := u.Forge.List(ctx, workspace)
	if err != nil {
		return PublishDraft{}, err
	}
	tool, found := findForged(installed, request.Name)
	if !found {
		return PublishDraft{}, fmt.Errorf("no tool named %s is registered in this workspace; /tools lists them", request.Name)
	}
	if tool.Origin != nil {
		return PublishDraft{}, fmt.Errorf("%s was installed from catalog %s; only a tool forged here is published", tool.Name, tool.Origin.Catalog)
	}
	// The files published are the files that were approved: the registry
	// refuses a tool whose files changed since it was forged.
	if err := u.Forge.Verify(ctx, workspace, tool); err != nil {
		return PublishDraft{}, err
	}
	draft := PublishDraft{Catalog: state.Name, API: state.APIURL, Tool: tool, Files: map[string][]byte{}}
	for name := range tool.Files {
		if !publishFileName.MatchString(name) || name == "." || name == ".." {
			return PublishDraft{}, fmt.Errorf("file %s of %s is not a bare file name; a registry takes files without a path, so forge the tool with its files at the top of its directory", name, tool.Name)
		}
		if name == PublishTestsFile || name == PublishCatalogFile {
			continue
		}
		content, err := u.Files.ReadToolFile(ctx, workspace, tool.Name, name)
		if err != nil {
			return PublishDraft{}, fmt.Errorf("file %s of %s: %v", name, tool.Name, err)
		}
		draft.Files[name] = content
	}
	if len(draft.Files) == 0 {
		return PublishDraft{}, fmt.Errorf("%s has no file to publish beside %s", tool.Name, PublishTestsFile)
	}
	tests, err := u.Files.ReadToolFile(ctx, workspace, tool.Name, PublishTestsFile)
	switch {
	case errors.Is(err, ErrNoToolFile):
		draft.Warnings = append(draft.Warnings, fmt.Sprintf("%s/%s/%s is missing: the registry accepts the version but cannot approve it until a version with tests is published", ForgedToolsDir, tool.Name, PublishTestsFile))
	case err != nil:
		return PublishDraft{}, fmt.Errorf("%s of %s: %v", PublishTestsFile, tool.Name, err)
	default:
		if err := checkTests(tests); err != nil {
			return PublishDraft{}, fmt.Errorf("%s/%s/%s: %v", ForgedToolsDir, tool.Name, PublishTestsFile, err)
		}
		draft.Tests = tests
	}
	stored, err := u.Files.ReadToolFile(ctx, workspace, tool.Name, PublishCatalogFile)
	switch {
	case errors.Is(err, ErrNoToolFile):
	case err != nil:
		return PublishDraft{}, fmt.Errorf("%s of %s: %v", PublishCatalogFile, tool.Name, err)
	default:
		decoder := json.NewDecoder(strings.NewReader(string(stored)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&draft.Metadata); err != nil {
			return PublishDraft{}, fmt.Errorf("%s/%s/%s is not a catalog block (version, category, tags, license, requires, network, example, source): %v", ForgedToolsDir, tool.Name, PublishCatalogFile, err)
		}
	}
	if request.Version != "" {
		draft.Metadata.Version = request.Version
	}
	if draft.Metadata.Tags == nil {
		draft.Metadata.Tags = []string{}
	}
	if draft.Metadata.Requires.Packages == nil {
		draft.Metadata.Requires.Packages = []string{}
	}
	draft.Missing = missingPublishFields(draft.Metadata)
	return draft, nil
}

// missingPublishFields lists the catalog fields that are empty or invalid,
// in the order they are asked. Tags may be empty once they were set:
// catalog.json carries them as an empty list then.
func missingPublishFields(meta PublishMetadata) []string {
	var missing []string
	if !catalogVersion.MatchString(meta.Version) {
		missing = append(missing, "version")
	}
	if !publishCategory.MatchString(meta.Category) {
		missing = append(missing, "category")
	}
	if !publishLicense.MatchString(meta.License) {
		missing = append(missing, "license")
	}
	for _, tag := range meta.Tags {
		if !publishTag.MatchString(tag) {
			missing = append(missing, "tags")
			break
		}
	}
	return missing
}

// SetPublishField takes the answer to one missing field and validates it;
// tags are separated by commas or spaces.
func SetPublishField(draft *PublishDraft, field, value string) error {
	value = strings.TrimSpace(value)
	switch field {
	case "version":
		if !catalogVersion.MatchString(value) {
			return errors.New("version must be major.minor.patch, such as 1.0.0")
		}
		draft.Metadata.Version = value
	case "category":
		if !publishCategory.MatchString(value) {
			return errors.New("category must be 1-48 lowercase letters, digits, _ or -, such as administration")
		}
		draft.Metadata.Category = value
	case "license":
		if !publishLicense.MatchString(value) {
			return errors.New("license must be an SPDX id, such as MIT or Apache-2.0")
		}
		draft.Metadata.License = value
	case "tags":
		tags := []string{}
		for _, tag := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' }) {
			if !publishTag.MatchString(tag) {
				return fmt.Errorf("tag %q must be 1-32 lowercase letters, digits, _ or -", tag)
			}
			tags = append(tags, tag)
		}
		if len(tags) > 16 {
			return errors.New("at most 16 tags")
		}
		draft.Metadata.Tags = tags
	default:
		return fmt.Errorf("no catalog field is named %s", field)
	}
	draft.Missing = missingPublishFields(draft.Metadata)
	return nil
}

// SaveMetadata writes the draft's catalog block to the tool's catalog.json.
func (u CatalogUseCase) SaveMetadata(ctx context.Context, workspace string, draft PublishDraft) error {
	if u.Files == nil {
		return errors.New("publishing to a registry is not enabled in this console")
	}
	data, err := json.MarshalIndent(draft.Metadata, "", "  ")
	if err != nil {
		return err
	}
	return u.Files.WriteToolFile(ctx, workspace, draft.Tool.Name, PublishCatalogFile, append(data, '\n'))
}

// Bundle builds the signed upload of a draft whose fields are complete:
// the manifest with the args named by their bare file names, as the
// registry and other consoles expect, the bundle digest and its signature
// with the client key, created when there is none.
func (u CatalogUseCase) Bundle(ctx context.Context, draft PublishDraft) (PublishBundle, error) {
	if !u.Publishable() {
		return PublishBundle{}, errors.New("publishing to a registry is not enabled in this console")
	}
	if missing := missingPublishFields(draft.Metadata); len(missing) > 0 {
		return PublishBundle{}, fmt.Errorf("the catalog block still lacks %s", strings.Join(missing, ", "))
	}
	if len(draft.Files) == 0 {
		return PublishBundle{}, errors.New("nothing to publish: the draft has no file")
	}
	own := ForgedToolsDir + "/" + draft.Tool.Name + "/"
	names := make([]string, 0, len(draft.Files))
	for name := range draft.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	args := make([]string, 0, len(draft.Tool.Args))
	for _, arg := range draft.Tool.Args {
		if bare, ok := strings.CutPrefix(path.Clean(arg), own); ok {
			if _, listed := draft.Files[bare]; listed {
				arg = bare
			}
		}
		args = append(args, arg)
	}
	manifest := publishManifest{Name: draft.Tool.Name, Description: draft.Tool.Description, InputSchema: draft.Tool.InputSchema, Program: draft.Tool.Program, Args: args, Files: names, Catalog: draft.Metadata}
	if manifest.Args == nil {
		manifest.Args = []string{}
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return PublishBundle{}, err
	}
	encoded = append(encoded, '\n')
	key, created, err := u.clientKey(ctx)
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

// Submit uploads a bundle to the draft's registry.
func (u CatalogUseCase) Submit(ctx context.Context, draft PublishDraft, bundle PublishBundle) (VersionReport, error) {
	if !u.Publishable() {
		return VersionReport{}, errors.New("publishing to a registry is not enabled in this console")
	}
	key, _, err := u.clientKey(ctx)
	if err != nil {
		return VersionReport{}, err
	}
	if key.ID() != bundle.KeyID {
		return VersionReport{}, errors.New("the client key changed since the bundle was signed; publish again")
	}
	return u.Registry.Submit(ctx, draft.API, key, bundle.Upload)
}

// Poll reads the draft's version from the registry.
func (u CatalogUseCase) Poll(ctx context.Context, draft PublishDraft) (VersionReport, error) {
	if !u.Publishable() {
		return VersionReport{}, errors.New("publishing to a registry is not enabled in this console")
	}
	key, _, err := u.clientKey(ctx)
	if err != nil {
		return VersionReport{}, err
	}
	return u.Registry.Version(ctx, draft.API, key, draft.Tool.Name, draft.Metadata.Version)
}

// Enroll sends the enrolment of this console's key for a publisher id to
// a catalog's registry; the catalog resolved is returned with the answer.
func (u CatalogUseCase) Enroll(ctx context.Context, catalog string, request EnrollRequest) (EnrollResult, string, error) {
	if !u.Publishable() {
		return EnrollResult{}, "", errors.New("publishing to a registry is not enabled in this console")
	}
	if !publisherID.MatchString(request.ID) {
		return EnrollResult{}, "", errors.New("publisher id must be 2-32 lowercase letters, digits, _ or -, starting with a letter")
	}
	if len(request.Namespaces) == 0 {
		return EnrollResult{}, "", errors.New("name at least one namespace, such as acme_*")
	}
	for _, namespace := range request.Namespaces {
		if !namespacePattern.MatchString(namespace) {
			return EnrollResult{}, "", fmt.Errorf("namespace %q must be lowercase letters, digits and underscores, with * at the end at most", namespace)
		}
	}
	catalogs, err := u.Catalog.Cached(ctx)
	if err != nil {
		return EnrollResult{}, "", err
	}
	state, err := resolveRegistry(catalogs, catalog)
	if err != nil {
		return EnrollResult{}, "", err
	}
	key, _, err := u.clientKey(ctx)
	if err != nil {
		return EnrollResult{}, "", err
	}
	result, err := u.Registry.Enroll(ctx, state.APIURL, key, request)
	return result, state.Name, err
}

var namespacePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*\*?$`)

// resolveRegistry picks the catalog to publish to: the one named, or the
// only one with an api_url.
func resolveRegistry(catalogs []CatalogState, name string) (CatalogState, error) {
	if name != "" {
		for _, state := range catalogs {
			if state.Name != name {
				continue
			}
			if state.APIURL == "" {
				return CatalogState{}, fmt.Errorf("catalog %s has no api_url in settings; a catalog without a registry takes no uploads", name)
			}
			return state, nil
		}
		return CatalogState{}, fmt.Errorf("no catalog is named %s; settings list the catalogs", name)
	}
	var candidates []CatalogState
	for _, state := range catalogs {
		if state.APIURL != "" {
			candidates = append(candidates, state)
		}
	}
	switch len(candidates) {
	case 0:
		return CatalogState{}, errors.New("no catalog has an api_url in settings; add one to the catalog that takes your tools")
	case 1:
		return candidates[0], nil
	default:
		names := make([]string, 0, len(candidates))
		for _, state := range candidates {
			names = append(names, state.Name)
		}
		return CatalogState{}, fmt.Errorf("catalogs %s have a registry; name one with --to", strings.Join(names, ", "))
	}
}

// checkTests reads tests.json as the registry's runner does: a non-empty
// list of cases with distinct names and object inputs.
func checkTests(data []byte) error {
	var cases []struct {
		Name   string          `json:"name"`
		Input  json.RawMessage `json:"input"`
		Expect json.RawMessage `json:"expect"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		return fmt.Errorf("not a JSON list of cases: %v", err)
	}
	if len(cases) == 0 {
		return errors.New("no cases")
	}
	seen := map[string]bool{}
	for _, c := range cases {
		if strings.TrimSpace(c.Name) == "" {
			return errors.New("a case has no name")
		}
		if seen[c.Name] {
			return fmt.Errorf("two cases are named %q", c.Name)
		}
		seen[c.Name] = true
		var input map[string]json.RawMessage
		if json.Unmarshal(c.Input, &input) != nil || input == nil {
			return fmt.Errorf("case %q needs an object input", c.Name)
		}
	}
	return nil
}
