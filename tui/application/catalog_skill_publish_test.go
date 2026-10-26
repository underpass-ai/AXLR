package application

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// skillSourcesMemory is an in-memory SkillSourcePort: each skill's files
// by name.
type skillSourcesMemory struct {
	skills map[string]map[string][]byte
}

func (s *skillSourcesMemory) ReadSkillSource(_ context.Context, _, name string) (map[string][]byte, error) {
	files, ok := s.skills[name]
	if !ok {
		return nil, ErrNoSkillSource
	}
	out := map[string][]byte{}
	for file, content := range files {
		out[file] = content
	}
	return out, nil
}

const triageSkillMD = "---\nname: incident_triage\ndescription: \"Triage a production incident from its logs: timeline, blast radius, first hypothesis.\"\ntools: [local_read, 'local_search', github/create_issue]\n---\n# Incident triage\n\nRead [the checklist](checklist.md) first.\n"

// skillPublishFixture is a publishing use case over the axlr catalog with
// a registry, and a workspace skill incident_triage with SKILL.md,
// checklist.md, catalog.json and tests.json.
func skillPublishFixture() (SkillCatalogUseCase, *skillSourcesMemory, *registryMemory, *keysMemory) {
	catalog := newCatalogMemory()
	catalog.api = "https://api.staging.example"
	sources := &skillSourcesMemory{skills: map[string]map[string][]byte{"incident_triage": {
		"SKILL.md":     []byte(triageSkillMD),
		"checklist.md": []byte("- logs\n- deploys\n"),
		"catalog.json": []byte(`{"version":"1.0.0","category":"programming","tags":["sre","incident"],"license":"MIT","source":"original"}`),
		"tests.json":   []byte(`[{"name":"starts_from_logs","prompt":"The API answers 502 since 10:00","expect":{"tools":["local_read"],"answer_contains":["timeline"]}}]`),
	}}}
	registry := &registryMemory{}
	keys := &keysMemory{}
	return SkillCatalogUseCase{Catalog: catalog, Keys: keys, Registry: registry, Sources: sources}, sources, registry, keys
}

// Prepare reads the workspace's skill as the registry will: SKILL.md's
// front matter gives the description and the tools (inline or block
// list), catalog.json the catalog block, tests.json travels beside; the
// bundle is the skill's own files.
func TestPrepareSkillReadsTheWorkspacesSkill(t *testing.T) {
	ctx := context.Background()
	use, sources, _, _ := skillPublishFixture()
	draft, err := use.PrepareSkill(ctx, "/ws", PublishRequest{Name: "incident_triage"})
	if err != nil {
		t.Fatal(err)
	}
	if draft.Catalog != "axlr" || draft.API != "https://api.staging.example" || draft.Name != "incident_triage" || draft.Description != "Triage a production incident from its logs: timeline, blast radius, first hypothesis." {
		t.Fatalf("draft %+v", draft)
	}
	if strings.Join(draft.Tools, ",") != "local_read,local_search,github/create_issue" || len(draft.Files) != 2 || draft.Files["checklist.md"] == nil || !bytes.Contains(draft.Tests, []byte("starts_from_logs")) {
		t.Fatalf("tools %v files %d tests %s", draft.Tools, len(draft.Files), draft.Tests)
	}
	if draft.Metadata.Version != "1.0.0" || draft.Metadata.Category != "programming" || strings.Join(draft.Metadata.Tags, ",") != "sre,incident" || draft.Metadata.License != "MIT" || draft.Metadata.Source != "original" {
		t.Fatalf("metadata %+v", draft.Metadata)
	}
	// The version asked for replaces catalog.json's.
	if draft, err := use.PrepareSkill(ctx, "/ws", PublishRequest{Name: "incident_triage", Version: "1.1.0"}); err != nil || draft.Metadata.Version != "1.1.0" {
		t.Fatalf("%+v %v", draft.Metadata, err)
	}
	// A block list of tools, no name in the front matter, no tests.
	skill := sources.skills["incident_triage"]
	skill["SKILL.md"] = []byte("---\ndescription: Triage an incident.\ntools:\n  - local_read\n  - \"log_summarise\"\nlicense: MIT\n---\n# Triage\n")
	delete(skill, "tests.json")
	draft, err = use.PrepareSkill(ctx, "/ws", PublishRequest{Name: "incident_triage"})
	if err != nil || strings.Join(draft.Tools, ",") != "local_read,log_summarise" || draft.Description != "Triage an incident." || draft.Tests != nil {
		t.Fatalf("%+v %v", draft, err)
	}
	// No tools at all is a skill that expects none.
	skill["SKILL.md"] = []byte("---\ndescription: Triage an incident.\n---\n# Triage\n")
	if draft, err := use.PrepareSkill(ctx, "/ws", PublishRequest{Name: "incident_triage"}); err != nil || len(draft.Tools) != 0 {
		t.Fatalf("%+v %v", draft.Tools, err)
	}
}

// What the registry would refuse is refused before anything is signed.
func TestPrepareSkillRefusals(t *testing.T) {
	ctx := context.Background()
	for name, tc := range map[string]struct {
		change  func(map[string][]byte)
		request PublishRequest
		want    string
	}{
		"name mismatch":   {func(s map[string][]byte) { s["SKILL.md"] = []byte("---\nname: triage\ndescription: x\n---\n") }, PublishRequest{}, `names the skill "triage"`},
		"no description":  {func(s map[string][]byte) { s["SKILL.md"] = []byte("---\nname: incident_triage\n---\n# Triage\n") }, PublishRequest{}, "needs a description"},
		"no front matter": {func(s map[string][]byte) { s["SKILL.md"] = []byte("# Triage\ndescription: in the body\n") }, PublishRequest{}, "needs a description"},
		"long description": {func(s map[string][]byte) {
			s["SKILL.md"] = []byte("---\ndescription: " + strings.Repeat("x", 1025) + "\n---\n")
		}, PublishRequest{}, "needs a description"},
		"no SKILL.md":     {func(s map[string][]byte) { delete(s, "SKILL.md") }, PublishRequest{}, "has no SKILL.md"},
		"no catalog.json": {func(s map[string][]byte) { delete(s, "catalog.json") }, PublishRequest{}, "catalog.json is missing"},
		"tool field in block": {func(s map[string][]byte) {
			s["catalog.json"] = []byte(`{"version":"1.0.0","category":"programming","tags":[],"license":"MIT","network":true}`)
		}, PublishRequest{}, "not a skill's catalog block"},
		"requires in block": {func(s map[string][]byte) {
			s["catalog.json"] = []byte(`{"version":"1.0.0","category":"programming","tags":[],"license":"MIT","requires":{"packages":[]}}`)
		}, PublishRequest{}, "not a skill's catalog block"},
		"bad catalog version": {func(s map[string][]byte) {
			s["catalog.json"] = []byte(`{"version":"one","category":"programming","tags":[],"license":"MIT"}`)
		}, PublishRequest{}, "version (major.minor.patch)"},
		"bad tag": {func(s map[string][]byte) {
			s["catalog.json"] = []byte(`{"version":"1.0.0","category":"programming","tags":["SRE"],"license":"MIT"}`)
		}, PublishRequest{}, `tag "SRE"`},
		"no license": {func(s map[string][]byte) {
			s["catalog.json"] = []byte(`{"version":"1.0.0","category":"programming","tags":[]}`)
		}, PublishRequest{}, "license"},
		"tests not a list": {func(s map[string][]byte) { s["tests.json"] = []byte(`{"name":"a"}`) }, PublishRequest{}, "tests.json: not a list of cases"},
		"tests empty":      {func(s map[string][]byte) { s["tests.json"] = []byte(`[]`) }, PublishRequest{}, "tests.json: no cases"},
		"tests no prompt":  {func(s map[string][]byte) { s["tests.json"] = []byte(`[{"name":"a","expect":{}}]`) }, PublishRequest{}, "every case has a name and a prompt"},
		"tests twice": {func(s map[string][]byte) {
			s["tests.json"] = []byte(`[{"name":"a","prompt":"p"},{"name":"a","prompt":"q"}]`)
		}, PublishRequest{}, `two cases are named "a"`},
		"tests unknown key": {func(s map[string][]byte) {
			s["tests.json"] = []byte(`[{"name":"a","prompt":"p","expect":{"stdout_json":{}}}]`)
		}, PublishRequest{}, "not a list of cases"},
		"tests trailing": {func(s map[string][]byte) { s["tests.json"] = []byte(`[{"name":"a","prompt":"p"}] []`) }, PublishRequest{}, "trailing data"},
		"nine files": {func(s map[string][]byte) {
			for i := range 7 {
				s[fmt.Sprintf("ref%d.md", i)] = []byte("ref\n")
			}
		}, PublishRequest{}, "at most eight files"},
		"empty file":  {func(s map[string][]byte) { s["empty.md"] = nil }, PublishRequest{}, "1 byte to 256 KiB"},
		"binary file": {func(s map[string][]byte) { s["blob.md"] = []byte("a\x00b") }, PublishRequest{}, "1 byte to 256 KiB"},
		"hidden file": {func(s map[string][]byte) { s[".env"] = []byte("KEY=1\n") }, PublishRequest{}, "not a flat name"},
		"bad tool":    {func(s map[string][]byte) { s["SKILL.md"] = []byte("---\ndescription: x\ntools: [Local_Read]\n---\n") }, PublishRequest{}, `tool "Local_Read"`},
		"tool twice": {func(s map[string][]byte) {
			s["SKILL.md"] = []byte("---\ndescription: x\ntools: [local_read, local_read]\n---\n")
		}, PublishRequest{}, "listed twice"},
		"bad request name": {func(map[string][]byte) {}, PublishRequest{Name: "Incident"}, "name must be"},
		"bad request ver":  {func(map[string][]byte) {}, PublishRequest{Version: "1.0"}, "version must be"},
		"unknown catalog":  {func(map[string][]byte) {}, PublishRequest{Catalog: "acme"}, "no catalog is named acme"},
		"not in workspace": {func(map[string][]byte) {}, PublishRequest{Name: "postmortem"}, ErrNoSkillSource.Error()},
	} {
		use, sources, registry, keys := skillPublishFixture()
		tc.change(sources.skills["incident_triage"])
		request := tc.request
		if request.Name == "" {
			request.Name = "incident_triage"
		}
		_, err := use.PrepareSkill(ctx, "/ws", request)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: want %q, got %v", name, tc.want, err)
		}
		if len(registry.uploads) != 0 || keys.creates != 0 {
			t.Fatalf("%s: a refused skill reached the registry or the key", name)
		}
	}
	var tools []string
	for i := range 33 {
		tools = append(tools, fmt.Sprintf("tool_%02d", i))
	}
	use, sources, _, _ := skillPublishFixture()
	sources.skills["incident_triage"]["SKILL.md"] = []byte("---\ndescription: x\ntools: [" + strings.Join(tools, ", ") + "]\n---\n")
	if _, err := use.PrepareSkill(ctx, "/ws", PublishRequest{Name: "incident_triage"}); err == nil || !strings.Contains(err.Error(), "at most 32 tools") {
		t.Fatalf("33 tools: %v", err)
	}
	if _, err := (SkillCatalogUseCase{Catalog: use.Catalog}).PrepareSkill(ctx, "/ws", PublishRequest{Name: "incident_triage"}); err == nil || !strings.Contains(err.Error(), "not enabled") {
		t.Fatalf("publishing off: %v", err)
	}
	if !errors.Is(func() error {
		_, err := use.PrepareSkill(ctx, "/ws", PublishRequest{Name: "postmortem"})
		return err
	}(), ErrNoSkillSource) {
		t.Fatal("a missing source is not ErrNoSkillSource")
	}
}

// The bundle is the skill's manifest, keys in the registry's order with
// the files sorted and empty lists written as [], digested as the
// registry digests a bundle and signed with the client key, created on the
// spot; the upload carries tests.json beside and verifies at the registry.
func TestBundleSkillWritesAndSignsTheManifest(t *testing.T) {
	ctx := context.Background()
	use, sources, registry, keys := skillPublishFixture()
	draft, err := use.PrepareSkill(ctx, "/ws", PublishRequest{Name: "incident_triage"})
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := use.BundleSkill(ctx, draft)
	if err != nil || !bundle.KeyCreated || keys.creates != 1 || bundle.KeyID != keys.key.ID() || bundle.Upload.KeyID != keys.key.ID() {
		t.Fatalf("%+v %v", bundle, err)
	}
	manifest := string(bundle.Upload.Manifest)
	want := `{
  "kind": "skill",
  "name": "incident_triage",
  "description": "Triage a production incident from its logs: timeline, blast radius, first hypothesis.",
  "files": [
    "SKILL.md",
    "checklist.md"
  ],
  "tools": [
    "local_read",
    "local_search",
    "github/create_issue"
  ],
  "catalog": {
    "version": "1.0.0",
    "category": "programming",
    "tags": [
      "sre",
      "incident"
    ],
    "license": "MIT",
    "source": "original"
  }
}
`
	if manifest != want {
		t.Fatalf("manifest:\n%s", manifest)
	}
	if bundle.Digest != BundleDigest(bundle.Upload.Manifest, draft.Files) || len(bundle.Upload.Files) != 2 || !bytes.Equal(bundle.Upload.Tests, draft.Tests) {
		t.Fatalf("digest %s, files %d", bundle.Digest, len(bundle.Upload.Files))
	}
	if !ed25519.Verify(keys.key.Public, BundleSignatureMessage(bundle.Digest), bundle.Upload.Signature) {
		t.Fatal("the bundle signature does not verify")
	}
	report, err := use.SubmitSkill(ctx, draft, bundle)
	if err != nil || report.Status != "scanned" || len(registry.uploads) != 1 || registry.api != "https://api.staging.example" {
		t.Fatalf("%+v %v", report, err)
	}
	registry.report = VersionReport{Name: "incident_triage", Version: "1.0.0", Status: "approved"}
	if polled, err := use.PollSkill(ctx, draft); err != nil || polled.Status != "approved" || registry.polls != 1 {
		t.Fatalf("%+v %v", polled, err)
	}
	// A second bundle reuses the key; no tools and no tags are [] and no
	// source is left out.
	sources.skills["incident_triage"]["SKILL.md"] = []byte("---\ndescription: Triage an incident.\n---\n# Triage\n")
	sources.skills["incident_triage"]["catalog.json"] = []byte(`{"version":"1.0.1","category":"programming","license":"MIT"}`)
	draft, err = use.PrepareSkill(ctx, "/ws", PublishRequest{Name: "incident_triage"})
	if err != nil {
		t.Fatal(err)
	}
	again, err := use.BundleSkill(ctx, draft)
	if err != nil || again.KeyCreated || keys.creates != 1 {
		t.Fatalf("%+v %v", again, err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(again.Upload.Manifest, &decoded); err != nil || string(decoded["tools"]) != "[]" || !strings.Contains(string(decoded["catalog"]), `"tags": []`) || strings.Contains(string(decoded["catalog"]), "source") {
		t.Fatalf("%s: %v", again.Upload.Manifest, err)
	}
	// A key changed between the bundle and the upload refuses the upload.
	keys.key = nil
	if _, err := keys.Create(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := use.SubmitSkill(ctx, draft, again); err == nil || !strings.Contains(err.Error(), "client key changed") {
		t.Fatalf("%v", err)
	}
}
