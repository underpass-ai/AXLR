package axlrplugin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The console ships no built-in skill since 13 October 2026: the index
// lists none, the axlr and made namespaces refuse every read and a copied
// package cannot pose as one of them.
func TestBuiltinNamespacesHoldNoSkillAndCannotBeShadowed(t *testing.T) {
	catalog := Catalog{Root: t.TempDir()}
	ctx := context.Background()
	guidance, err := catalog.Guidance(ctx)
	if err != nil || strings.Contains(guidance, "axlr:") || strings.Contains(guidance, "axlr-session") || strings.Contains(guidance, "axlr-ceremonies") {
		t.Fatalf("built-in skill index: %q %v", guidance, err)
	}
	entries, err := os.ReadDir(catalog.Root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("reading defaults mutated package storage: %v %v", entries, err)
	}
	mustWrite(t, filepath.Join(catalog.Root, "plugins", "installed", "axlr", "skills", "axlr-session", "SKILL.md"), "shadowed")
	for _, entry := range []struct{ plugin, skill, path string }{{"axlr", "axlr-session", "SKILL.md"}, {"axlr", "axlr-session", "references/interabouts.md"}, {"made", "axlr-ceremonies", "SKILL.md"}, {"axlr", "other", "SKILL.md"}} {
		if _, err := catalog.ReadSkill(ctx, entry.plugin, entry.skill, entry.path, 0, 4096); err == nil {
			t.Fatalf("served a built-in skill: %+v", entry)
		}
	}
	if err := catalog.Install(ctx, "axlr"); err == nil {
		t.Fatal("built-in namespace accepted package installation")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := catalog.Guidance(cancelled); err == nil {
		t.Fatal("canceled discovery succeeded")
	}
}

// Skill pages keep UTF-8 characters whole and make progress.
func TestSkillPagesKeepCharactersWhole(t *testing.T) {
	data := []byte(strings.Repeat("sesión · título — ", 40))
	var got strings.Builder
	for offset := 0; ; {
		page, err := skillPage("p", "s", "SKILL.md", data, offset, 37)
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
	if got.String() != string(data) {
		t.Fatal("paging lost bytes")
	}
	if _, err := skillPage("p", "s", "SKILL.md", data, 5, 37); err == nil {
		t.Fatal("an offset inside a character was accepted")
	}
	if _, err := skillPage("p", "s", "SKILL.md", data, len(data)+1, 37); err == nil {
		t.Fatal("an offset past the data was accepted")
	}
}
