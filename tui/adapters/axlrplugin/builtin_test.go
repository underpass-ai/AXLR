package axlrplugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultCeremonySkillNeedsNoPackageOrEngine(t *testing.T) {
	catalog := Catalog{Root: t.TempDir()}
	ctx := context.Background()
	guidance, err := catalog.Guidance(ctx)
	if err != nil || !strings.Contains(guidance, "made:axlr-ceremonies") {
		t.Fatalf("default skill index: %q %v", guidance, err)
	}
	text := readAllSkill(t, &catalog, "SKILL.md")
	for _, name := range []string{"axlr_change", "axlr_delivery", "axlr_debug", "axlr_review", "axlr_research", "axlr_publish", "axlr_handoff"} {
		if !strings.Contains(text, name) {
			t.Fatalf("default route %s is absent", name)
		}
	}
	entries, err := os.ReadDir(catalog.Root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("reading defaults mutated package storage: %v %v", entries, err)
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := catalog.Guidance(ctx); err == nil {
		t.Fatal("canceled discovery succeeded")
	}
}

func TestDefaultCatalogPinsEveryReadableDefinition(t *testing.T) {
	catalog := Catalog{Root: t.TempDir()}
	var manifest struct {
		Minimum string `json:"minimum_made_version"`
		Entries []struct {
			Name   string `json:"name"`
			File   string `json:"file"`
			SHA    string `json:"file_sha256"`
			Digest string `json:"definition_digest"`
		} `json:"ceremonies"`
	}
	if err := json.Unmarshal([]byte(readAllSkill(t, &catalog, "references/catalog.json")), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Minimum != "0.9.1" || len(manifest.Entries) != 7 {
		t.Fatalf("default catalogue: %+v", manifest)
	}
	for _, entry := range manifest.Entries {
		data := readAllSkill(t, &catalog, "references/"+entry.File)
		digest := sha256.Sum256([]byte(data))
		if entry.SHA != hex.EncodeToString(digest[:]) || len(entry.Digest) != 64 || !strings.Contains(data, "name: "+entry.Name+"\n") {
			t.Fatalf("definition identity is stale: %+v", entry)
		}
	}
	for _, resource := range []string{"references/execution.md", "references/handoff.md", "references/research.md", "agents/openai.yaml"} {
		if readAllSkill(t, &catalog, resource) == "" {
			t.Fatalf("empty supporting resource: %s", resource)
		}
	}
}

func TestBuiltinSkillCannotReadOtherFilesOrBeShadowed(t *testing.T) {
	catalog := Catalog{Root: t.TempDir()}
	mustWrite(t, filepath.Join(catalog.Root, "plugins", "installed", "made", "skills", builtinCeremonySkill, "SKILL.md"), "shadowed")
	if strings.Contains(readAllSkill(t, &catalog, "SKILL.md"), "shadowed") {
		t.Fatal("filesystem package shadowed the built-in skill")
	}
	for _, resource := range []string{"../SKILL.md", "references/../../catalog.go", "/etc/passwd", "references//catalog.json", "references/../SKILL.md", "references/missing.yaml", ".private", "agents/other.yaml"} {
		if _, err := catalog.ReadSkill(context.Background(), "made", builtinCeremonySkill, resource, 0, 4096); err == nil {
			t.Fatalf("accepted unavailable resource %q", resource)
		}
	}
	if _, err := catalog.ReadSkill(context.Background(), "made", "other", "SKILL.md", 0, 4096); err == nil {
		t.Fatal("accepted unknown built-in skill")
	}
	if _, err := catalog.ReadSkill(context.Background(), "made", builtinCeremonySkill, "SKILL.md", 1<<20, 4096); err == nil {
		t.Fatal("accepted offset past resource")
	}
}

func TestSessionSkillPagesWithoutEngineOrPackageAndCannotBeShadowed(t *testing.T) {
	catalog := Catalog{Root: t.TempDir()}
	ctx := context.Background()
	guidance, err := catalog.Guidance(ctx)
	if err != nil || !strings.Contains(guidance, "axlr:axlr-session") {
		t.Fatalf("session skill undiscoverable: %s %v", guidance, err)
	}
	mustWrite(t, filepath.Join(catalog.Root, "plugins", "installed", "axlr", "skills", builtinSessionSkill, "SKILL.md"), "shadowed")
	for _, resource := range []string{"SKILL.md", "references/interabouts.md"} {
		var got strings.Builder
		for offset := 0; ; {
			page, err := catalog.ReadSkill(ctx, "axlr", builtinSessionSkill, resource, offset, 137)
			if err != nil {
				t.Fatal(err)
			}
			got.WriteString(page.Text)
			if !page.HasMore {
				break
			}
			if page.NextOffsetBytes <= offset {
				t.Fatal("paging stalled")
			}
			offset = page.NextOffsetBytes
		}
		want, err := builtinResources.ReadFile(builtinSessionRoot + resource)
		if err != nil || got.String() != string(want) {
			t.Fatalf("session skill page lost data: %s %v", resource, err)
		}
	}
	for _, entry := range []struct{ skill, path string }{{builtinCeremonySkill, "SKILL.md"}, {builtinSessionSkill, "../SKILL.md"}, {builtinSessionSkill, "references/catalog.json"}} {
		if _, err := catalog.ReadSkill(ctx, "axlr", entry.skill, entry.path, 0, 4096); err == nil {
			t.Fatalf("escaped session skill: %+v", entry)
		}
	}
	if err := catalog.Install(ctx, "axlr"); err == nil {
		t.Fatal("built-in namespace accepted package installation")
	}
}

func readAllSkill(t *testing.T, catalog *Catalog, resource string) string {
	t.Helper()
	var text strings.Builder
	for offset := 0; ; {
		page, err := catalog.ReadSkill(context.Background(), "made", builtinCeremonySkill, resource, offset, 257)
		if err != nil {
			t.Fatal(err)
		}
		text.WriteString(page.Text)
		if !page.HasMore {
			if text.Len() != page.TotalBytes {
				t.Fatal("resource paging lost bytes")
			}
			return text.String()
		}
		if page.NextOffsetBytes <= offset {
			t.Fatal("resource paging made no progress")
		}
		offset = page.NextOffsetBytes
	}
}
