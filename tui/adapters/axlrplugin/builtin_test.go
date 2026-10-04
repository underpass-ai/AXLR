package axlrplugin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltinGuidanceListsOnlyTheSessionSkill(t *testing.T) {
	catalog := Catalog{Root: t.TempDir()}
	ctx := context.Background()
	guidance, err := catalog.Guidance(ctx)
	if err != nil || !strings.Contains(guidance, "axlr:axlr-session") || strings.Contains(guidance, "axlr-ceremonies") {
		t.Fatalf("built-in skill index: %q %v", guidance, err)
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
	// The retired catalogue is gone from the built-in namespace, not merely
	// hidden: a request for it is refused like any unknown skill.
	for _, skill := range []string{"axlr-ceremonies", "other"} {
		if _, err := catalog.ReadSkill(context.Background(), "made", skill, "SKILL.md", 0, 4096); err == nil {
			t.Fatalf("accepted unknown built-in skill %s", skill)
		}
	}
}

func TestBuiltinSkillCannotReadOtherFilesOrBeShadowed(t *testing.T) {
	catalog := Catalog{Root: t.TempDir()}
	mustWrite(t, filepath.Join(catalog.Root, "plugins", "installed", "axlr", "skills", builtinSessionSkill, "SKILL.md"), "shadowed")
	if strings.Contains(readAllSkill(t, &catalog, "SKILL.md"), "shadowed") {
		t.Fatal("filesystem package shadowed the built-in skill")
	}
	for _, resource := range []string{"../SKILL.md", "references/../../catalog.go", "/etc/passwd", "references//interabouts.md", "references/../SKILL.md", "references/missing.md", ".private", "agents/other.yaml"} {
		if _, err := catalog.ReadSkill(context.Background(), "axlr", builtinSessionSkill, resource, 0, 4096); err == nil {
			t.Fatalf("accepted unavailable resource %q", resource)
		}
	}
	if _, err := catalog.ReadSkill(context.Background(), "axlr", builtinSessionSkill, "SKILL.md", 1<<20, 4096); err == nil {
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
	for _, entry := range []struct{ skill, path string }{{"axlr-ceremonies", "SKILL.md"}, {builtinSessionSkill, "../SKILL.md"}, {builtinSessionSkill, "references/catalog.json"}} {
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
		page, err := catalog.ReadSkill(context.Background(), "axlr", builtinSessionSkill, resource, offset, 257)
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
