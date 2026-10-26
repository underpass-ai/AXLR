package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// An execution's requested pins resolve against the installed versions:
// the catalog filled in when one catalog's install has the name, the
// bundle digest from the install; a pin this host cannot serve wraps
// ErrSkillsUnavailable and says what is installed.
func TestResolveSkillPins(t *testing.T) {
	ctx := context.Background()
	shelf := newSkillShelf()
	triage := shelf.put("axlr", "incident_triage", "1.0.0", map[string]string{"SKILL.md": "# One\n"})
	shelf.put("axlr", "incident_triage", "1.1.0", map[string]string{"SKILL.md": "# Two\n"})
	postmortem := shelf.put("acme", "postmortem", "2.0.0", map[string]string{"SKILL.md": "# Postmortem\n"})
	shelf.put("axlr", "postmortem", "2.0.0", map[string]string{"SKILL.md": "# Other postmortem\n"})
	skills, err := ResolveSkillPins(ctx, shelf, []SkillRequest{{Name: "incident_triage", Version: "1.0.0"}, {Name: "postmortem", Version: "2.0.0", Catalog: "acme"}})
	if err != nil || len(skills.Pins) != 2 {
		t.Fatalf("%+v %v", skills, err)
	}
	if skills.Pins[0] != (domain.SkillPin{Catalog: "axlr", Name: "incident_triage", Version: "1.0.0", BundleSHA256: triage.BundleSHA256}) || skills.Pins[1] != (domain.SkillPin{Catalog: "acme", Name: "postmortem", Version: "2.0.0", BundleSHA256: postmortem.BundleSHA256}) {
		t.Fatalf("pins %+v", skills.Pins)
	}
	for name, tc := range map[string]struct {
		requests []SkillRequest
		want     string
	}{
		"wrong version": {[]SkillRequest{{Name: "incident_triage", Version: "2.0.0"}}, "incident_triage 2.0.0 is not installed (installed: 1.0.0, 1.1.0)"},
		"not installed": {[]SkillRequest{{Name: "runbook", Version: "1.0.0"}}, "runbook is not installed"},
		"other catalog": {[]SkillRequest{{Name: "incident_triage", Version: "1.0.0", Catalog: "acme"}}, "incident_triage is not installed"},
		"two catalogs":  {[]SkillRequest{{Name: "postmortem", Version: "2.0.0"}}, "postmortem 2.0.0 is installed from catalogs acme, axlr; name the catalog"},
	} {
		_, err := ResolveSkillPins(ctx, shelf, tc.requests)
		if !errors.Is(err, domain.ErrSkillsUnavailable) || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: want %q wrapping ErrSkillsUnavailable, got %v", name, tc.want, err)
		}
	}
	if _, err := ResolveSkillPins(ctx, shelf, []SkillRequest{{Name: "incident_triage", Version: "1.0.0"}, {Name: "incident_triage", Version: "1.1.0"}}); err == nil || !strings.Contains(err.Error(), "pinned twice") {
		t.Fatalf("one skill pinned twice: %v", err)
	}
	// No request is an execution with no catalog skill, even where catalog
	// skills are off; a request there is unavailable.
	none, err := ResolveSkillPins(ctx, nil, nil)
	if err != nil || none.Pins == nil || len(none.Pins) != 0 {
		t.Fatalf("%+v %v", none, err)
	}
	if _, err := ResolveSkillPins(ctx, nil, []SkillRequest{{Name: "incident_triage", Version: "1.0.0"}}); !errors.Is(err, domain.ErrSkillsUnavailable) {
		t.Fatalf("pins without catalog skills: %v", err)
	}
}

// pinnedSession is a session pinned to skills.
func pinnedSession(t *testing.T, pins ...domain.SkillPin) domain.Session {
	t.Helper()
	s := turnSession(t)
	if err := s.SetSkills(domain.ExecutionSkills{Pins: pins}); err != nil {
		t.Fatal(err)
	}
	return s
}

// Without pins a session reads the newest installed version of each
// catalog's skill; pinned, exactly its pins, an older version even when a
// newer one is installed, nothing it does not pin, and a pinned version
// removed since is named as such.
func TestSessionSkillReaderServesThePinnedVersions(t *testing.T) {
	ctx := context.Background()
	shelf := newSkillShelf()
	old := shelf.put("axlr", "incident_triage", "1.9.0", map[string]string{"SKILL.md": "# Triage 1.9\n", "checklist.md": "- old\n"})
	shelf.put("axlr", "incident_triage", "1.10.0", map[string]string{"SKILL.md": "# Triage 1.10\n"})
	shelf.put("acme", "postmortem", "1.0.0", map[string]string{"SKILL.md": "# Postmortem\n"})
	reader := SessionSkillReader{Installed: shelf}
	keys := func(skills []InstalledSkill) string {
		var out []string
		for _, skill := range skills {
			out = append(out, skill.Key())
		}
		return strings.Join(out, " ")
	}
	// An unpinned session carries nothing and reads the newest.
	unpinned := WithSessionSkills(ctx, turnSession(t))
	if _, pinned := SessionSkillsOf(unpinned); pinned {
		t.Fatal("an unpinned session carried pins")
	}
	visible, err := reader.Visible(unpinned)
	if err != nil || keys(visible) != "acme/postmortem@1.0.0 axlr/incident_triage@1.10.0" {
		t.Fatalf("%s %v", keys(visible), err)
	}
	skill, data, err := reader.Read(unpinned, "axlr", "incident_triage", "SKILL.md")
	if err != nil || skill.Version != "1.10.0" || string(data) != "# Triage 1.10\n" {
		t.Fatalf("%+v %q %v", skill, data, err)
	}
	if _, _, err := reader.Read(unpinned, "axlr", "runbook", "SKILL.md"); err == nil || !strings.Contains(err.Error(), "no skill runbook is installed from catalog axlr") {
		t.Fatalf("%v", err)
	}
	// A session pinned to the older version reads it.
	pinned := WithSessionSkills(ctx, pinnedSession(t, domain.SkillPin{Catalog: "axlr", Name: "incident_triage", Version: "1.9.0", BundleSHA256: old.BundleSHA256}))
	if skills, has := SessionSkillsOf(pinned); !has || len(skills.Pins) != 1 {
		t.Fatalf("%+v %v", skills, has)
	}
	visible, err = reader.Visible(pinned)
	if err != nil || keys(visible) != "axlr/incident_triage@1.9.0" {
		t.Fatalf("%s %v", keys(visible), err)
	}
	skill, data, err = reader.Read(pinned, "axlr", "incident_triage", "checklist.md")
	if err != nil || skill.Version != "1.9.0" || string(data) != "- old\n" {
		t.Fatalf("%+v %q %v", skill, data, err)
	}
	if _, _, err := reader.Read(pinned, "acme", "postmortem", "SKILL.md"); err == nil || !strings.Contains(err.Error(), "this execution's skills do not include postmortem of catalog acme") {
		t.Fatalf("an unpinned skill was read: %v", err)
	}
	if _, err := shelf.Remove(ctx, "axlr", "incident_triage", "1.9.0"); err != nil {
		t.Fatal(err)
	}
	if visible, err := reader.Visible(pinned); err != nil || len(visible) != 0 {
		t.Fatalf("a removed pin is listed: %s %v", keys(visible), err)
	}
	if _, _, err := reader.Read(pinned, "axlr", "incident_triage", "SKILL.md"); err == nil || !strings.Contains(err.Error(), "incident_triage 1.9.0 of catalog axlr, which this execution pins, is no longer installed") {
		t.Fatalf("a removed pin: %v", err)
	}
	// Pinned to no skill: none is listed or read.
	nothing := WithSessionSkills(ctx, pinnedSession(t))
	if visible, err := reader.Visible(nothing); err != nil || len(visible) != 0 {
		t.Fatalf("%s %v", keys(visible), err)
	}
	if _, _, err := reader.Read(nothing, "acme", "postmortem", "SKILL.md"); err == nil {
		t.Fatal("a session pinned to no skill read one")
	}
	// Without the store there is nothing to list and nothing to read.
	if visible, err := (SessionSkillReader{}).Visible(ctx); err != nil || visible != nil {
		t.Fatalf("%+v %v", visible, err)
	}
	if _, _, err := (SessionSkillReader{}).Read(ctx, "acme", "postmortem", "SKILL.md"); err == nil {
		t.Fatal("read without a store")
	}
}

// catalogSkillPages serves both plugin skills and catalog skills,
// recording each request.
type catalogSkillPages struct {
	plugin, catalog []string
}

func (p *catalogSkillPages) ReadSkill(_ context.Context, plugin, skill, path string, offset, limit int) (SkillPage, error) {
	p.plugin = append(p.plugin, plugin+"/"+skill+"/"+path)
	return SkillPage{Plugin: plugin, Skill: skill, Path: path, Text: "plugin page"}, nil
}

func (p *catalogSkillPages) ReadCatalogSkill(_ context.Context, catalog, skill, path string, offset, limit int) (SkillPage, error) {
	p.catalog = append(p.catalog, catalog+"/"+skill+"/"+path)
	if offset != 0 || limit != 4096 && limit != 100 {
		return SkillPage{}, errors.New("unexpected range")
	}
	return SkillPage{Catalog: catalog, Version: "1.0.0", Skill: skill, Path: path, Text: "# Catalog page"}, nil
}

// axlr_skill names either plugin or catalog: catalog reads the catalog
// skill's page through the catalog page port, plugin the plugin's, and a
// console without catalog skills says so.
func TestHostSkillRoutesCatalogSkills(t *testing.T) {
	ctx := context.Background()
	id, err := domain.NewHostToolIdentity("skill")
	if err != nil {
		t.Fatal(err)
	}
	pages := &catalogSkillPages{}
	use := HostToolUseCase{Skills: pages}
	out, err := use.Execute(ctx, domain.Session{}, id, hostJSON(t, `{"catalog":"axlr","skill":"incident_triage"}`))
	if err != nil || out.IsError {
		t.Fatalf("%+v %v", out, err)
	}
	var page SkillPage
	if err := json.Unmarshal([]byte(out.Content), &page); err != nil || page.Catalog != "axlr" || page.Version != "1.0.0" || page.Plugin != "" || page.Text != "# Catalog page" {
		t.Fatalf("%s: %v", out.Content, err)
	}
	if strings.Contains(string(out.Content), `"plugin"`) {
		t.Fatalf("a catalog page names a plugin: %s", out.Content)
	}
	if out, err := use.Execute(ctx, domain.Session{}, id, hostJSON(t, `{"catalog":"axlr","skill":"incident_triage","path":"checklist.md","limit_bytes":100}`)); err != nil || out.IsError {
		t.Fatalf("%+v %v", out, err)
	}
	if out, err := use.Execute(ctx, domain.Session{}, id, hostJSON(t, `{"plugin":"visualization","skill":"chart"}`)); err != nil || out.IsError {
		t.Fatalf("%+v %v", out, err)
	}
	if strings.Join(pages.catalog, " ") != "axlr/incident_triage/SKILL.md axlr/incident_triage/checklist.md" || strings.Join(pages.plugin, " ") != "visualization/chart/SKILL.md" {
		t.Fatalf("catalog %v plugin %v", pages.catalog, pages.plugin)
	}
	for _, args := range []string{
		`{"skill":"incident_triage"}`,
		`{"plugin":"visualization","catalog":"axlr","skill":"incident_triage"}`,
		`{"catalog":"ACME","skill":"incident_triage"}`,
		`{"catalog":"../axlr","skill":"incident_triage"}`,
		`{"catalog":7,"skill":"incident_triage"}`,
		`{"catalog":"axlr","skill":"../incident_triage"}`,
		`{"catalog":"axlr","skill":"incident_triage","limit_bytes":5000}`,
		`{"catalog":"axlr","skill":"incident_triage","version":"1.0.0"}`,
	} {
		out, err := use.Execute(ctx, domain.Session{}, id, hostJSON(t, args))
		if err != nil || !out.IsError {
			t.Fatalf("accepted %s: %+v %v", args, out, err)
		}
	}
	if len(pages.catalog) != 2 || len(pages.plugin) != 1 {
		t.Fatalf("a refused request reached a reader: catalog %v plugin %v", pages.catalog, pages.plugin)
	}
	plugins := skillPortFunc(func(context.Context, string, string, string, int, int) (SkillPage, error) {
		t.Fatal("a catalog request reached the plugin reader")
		return SkillPage{}, nil
	})
	out, err = (HostToolUseCase{Skills: plugins}).Execute(ctx, domain.Session{}, id, hostJSON(t, `{"catalog":"axlr","skill":"incident_triage"}`))
	if err != nil || !out.IsError || !strings.Contains(string(out.Content), "catalog skills are unavailable") {
		t.Fatalf("%+v %v", out, err)
	}
}
