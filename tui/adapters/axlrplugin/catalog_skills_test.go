package axlrplugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// catalogSkillsFixture installs incident_triage 1.9.0 and 1.10.0 from
// catalog axlr and postmortem 1.0.0 from acme, and returns a plugin
// catalog with no plugin that serves them.
func catalogSkillsFixture(t *testing.T) (*Catalog, *storage.CatalogSkillStore) {
	t.Helper()
	store, err := storage.NewCatalogSkillStore(filepath.Join(t.TempDir(), "skills"))
	if err != nil {
		t.Fatal(err)
	}
	install := func(catalog, name, version, description string, files map[string]string) {
		record := application.InstalledSkill{Catalog: catalog, Name: name, Version: version, Description: description, Files: map[string]string{}, BundleSHA256: strings.Repeat("e", 64), InstalledAt: time.Now().UTC()}
		var list []application.SkillFile
		for path, content := range files {
			sum := sha256.Sum256([]byte(content))
			record.Files[path] = hex.EncodeToString(sum[:])
			list = append(list, application.SkillFile{Path: path, Content: []byte(content)})
		}
		if _, err := store.Install(context.Background(), record, list); err != nil {
			t.Fatal(err)
		}
	}
	install("axlr", "incident_triage", "1.9.0", "Triage a production incident.", map[string]string{"SKILL.md": "# Triage 1.9\n", "checklist.md": "- logs\n- deploys\n"})
	install("axlr", "incident_triage", "1.10.0", "Triage a production incident, with the blast radius.", map[string]string{"SKILL.md": "# Triage 1.10\n"})
	install("acme", "postmortem", "1.0.0", "Write the postmortem of an incident.", map[string]string{"SKILL.md": "# Postmortem\n"})
	return &Catalog{Root: t.TempDir(), CatalogSkills: &application.SessionSkillReader{Installed: store}}, store
}

func pinnedContext(t *testing.T, pins ...domain.SkillPin) context.Context {
	t.Helper()
	session, err := domain.NewSession("0123456789abcdef0123456789abcdef", domain.Workspace(t.TempDir()), "test/model")
	if err != nil {
		t.Fatal(err)
	}
	if err := session.SetSkills(domain.ExecutionSkills{Pins: pins}); err != nil {
		t.Fatal(err)
	}
	return application.WithSessionSkills(context.Background(), session)
}

// The skill index lists each catalog's installed skills after the plugin
// skills, one line per catalog, each skill with its version leading its
// description: the newest installed without pins, exactly the pinned
// versions with them. The text is the same on every turn.
func TestGuidanceListsTheSessionsCatalogSkills(t *testing.T) {
	plugins, _ := catalogSkillsFixture(t)
	ctx := context.Background()
	guidance, err := plugins.Guidance(ctx)
	if err != nil {
		t.Fatal(err)
	}
	acme := "\n- catalog acme: Open these with axlr_skill and catalog acme instead of plugin. Skills: postmortem (version 1.0.0: Write the postmortem of an incident.)."
	axlr := "\n- catalog axlr: Open these with axlr_skill and catalog axlr instead of plugin. Skills: incident_triage (version 1.10.0: Triage a production incident, with the blast radius.)."
	if !strings.Contains(guidance, acme+axlr) || strings.Contains(guidance, "1.9.0") || !strings.HasSuffix(guidance, "\n"+skillFirstRule) || !strings.Contains(guidance, "with the plugin (or catalog) and skill names") {
		t.Fatalf("guidance:\n%s", guidance)
	}
	if again, err := plugins.Guidance(ctx); err != nil || again != guidance {
		t.Fatalf("the index changed between turns: %v", err)
	}
	pinned, err := plugins.Guidance(pinnedContext(t, domain.SkillPin{Catalog: "axlr", Name: "incident_triage", Version: "1.9.0", BundleSHA256: strings.Repeat("e", 64)}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pinned, "\n- catalog axlr: Open these with axlr_skill and catalog axlr instead of plugin. Skills: incident_triage (version 1.9.0: Triage a production incident.).") || strings.Contains(pinned, "1.10.0") || strings.Contains(pinned, "catalog acme") {
		t.Fatalf("pinned guidance:\n%s", pinned)
	}
	none, err := plugins.Guidance(pinnedContext(t))
	if err != nil || strings.Contains(none, "- catalog ") {
		t.Fatalf("pinned to no skill:\n%s %v", none, err)
	}
	without, err := (&Catalog{Root: plugins.Root}).Guidance(ctx)
	if err != nil || strings.Contains(without, "- catalog ") {
		t.Fatalf("without catalog skills:\n%s %v", without, err)
	}
}

// axlr_skill's catalog pages come from the session's version, name their
// catalog and version and no plugin, page like a plugin skill's, and are
// read only when the file still matches its install.
func TestReadCatalogSkillServesTheSessionsVersion(t *testing.T) {
	plugins, store := catalogSkillsFixture(t)
	ctx := context.Background()
	page, err := plugins.ReadCatalogSkill(ctx, "axlr", "incident_triage", "SKILL.md", 0, 4096)
	if err != nil || page.Catalog != "axlr" || page.Version != "1.10.0" || page.Plugin != "" || page.Skill != "incident_triage" || page.Path != "SKILL.md" || page.Text != "# Triage 1.10\n" || page.HasMore {
		t.Fatalf("%+v %v", page, err)
	}
	pinned := pinnedContext(t, domain.SkillPin{Catalog: "axlr", Name: "incident_triage", Version: "1.9.0", BundleSHA256: strings.Repeat("e", 64)})
	page, err = plugins.ReadCatalogSkill(pinned, "axlr", "incident_triage", "checklist.md", 0, 7)
	if err != nil || page.Version != "1.9.0" || page.Text != "- logs\n" || !page.HasMore || page.NextOffsetBytes != 7 || page.TotalBytes != 17 {
		t.Fatalf("%+v %v", page, err)
	}
	page, err = plugins.ReadCatalogSkill(pinned, "axlr", "incident_triage", "checklist.md", 7, 4096)
	if err != nil || page.Text != "- deploys\n" || page.HasMore || page.OffsetBytes != 7 {
		t.Fatalf("%+v %v", page, err)
	}
	if _, err := plugins.ReadCatalogSkill(pinned, "acme", "postmortem", "SKILL.md", 0, 4096); err == nil || !strings.Contains(err.Error(), "do not include") {
		t.Fatalf("an unpinned skill was read: %v", err)
	}
	if _, err := plugins.ReadCatalogSkill(ctx, "axlr", "incident_triage", "checklist.md", 0, 4096); err == nil || !strings.Contains(err.Error(), "has no file checklist.md") {
		t.Fatalf("a file of another version was read: %v", err)
	}
	for _, bad := range [][2]int{{-1, 10}, {0, 0}, {0, 4097}} {
		if _, err := plugins.ReadCatalogSkill(ctx, "axlr", "incident_triage", "SKILL.md", bad[0], bad[1]); err == nil {
			t.Fatalf("range %v read", bad)
		}
	}
	if err := os.WriteFile(filepath.Join(store.Dir, "axlr", "incident_triage", "1.9.0", "SKILL.md"), []byte("# Changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := plugins.ReadCatalogSkill(pinned, "axlr", "incident_triage", "SKILL.md", 0, 4096); err == nil || !strings.Contains(err.Error(), "changed since it was installed") {
		t.Fatalf("a changed file was read: %v", err)
	}
	if _, err := (&Catalog{Root: plugins.Root}).ReadCatalogSkill(ctx, "axlr", "incident_triage", "SKILL.md", 0, 4096); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("without catalog skills: %v", err)
	}
}
