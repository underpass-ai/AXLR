package madesetup

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/mcpclient"
	"github.com/underpass-ai/AXLR/tui/adapters/ceremonyhost"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
)

type recordedCall struct {
	tool string
	env  []string
	args map[string]any
}

type fakeEngine struct {
	calls     []recordedCall
	existing  bool
	refuse    *refusal
	published map[string]bool
}

func (f *fakeEngine) call(_ context.Context, s mcpclient.Server, tool string, arguments map[string]any) (map[string]any, error) {
	f.calls = append(f.calls, recordedCall{tool, s.Env, arguments})
	if tool == "made_issue_authorization_grant" {
		if f.refuse != nil {
			return nil, f.refuse
		}
		return map[string]any{"existing": f.existing, "version": 4.0}, nil
	}
	switch tool {
	case "made_get_ceremony_definition":
		for _, d := range ceremonyhost.Definitions() {
			if d.Name == arguments["ceremony"] && f.published[d.Name] {
				return map[string]any{"digest": d.Digest}, nil
			}
		}
		return nil, &refusal{Code: "not_found", Message: "definition not found"}
	case "made_publish_ceremony_definition":
		if f.published == nil {
			f.published = map[string]bool{}
		}
		for _, d := range ceremonyhost.Definitions() {
			if text, _ := d.YAML(); text == arguments["definition_yaml"] {
				f.published[d.Name] = true
			}
		}
		return map[string]any{"outcome": "published"}, nil
	}
	return map[string]any{"definitions": []any{}}, nil
}

func madeConfig(t *testing.T, launcher string, env map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, "made.json")
	data, _ := json.Marshal(map[string]any{"manifest_version": 1, "id": "made", "command": filepath.Join(dir, "scripts", launcher), "args": []string{}, "allow_tools": []string{"*"}})
	if err := os.WriteFile(manifest, data, 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "mcp.json")
	data, _ = json.Marshal(map[string]any{"version": 1, "plugins": []any{map[string]any{
		"manifest": manifest, "name": "MADE", "purpose": "ceremony", "approval": "manual", "env": env, "env_from": map[string]string{"HOME": "HOME"},
	}}})
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func prepare(t *testing.T, path string, engine *fakeEngine) (*storage.MCPConfigStore, func() (string, error)) {
	t.Helper()
	store := &storage.MCPConfigStore{Path: path}
	p := &Preparer{ConfigPath: path, Getenv: func(string) string { return "/home/test" }, Store: store, Call: engine.call}
	return store, func() (string, error) {
		r, err := p.Prepare(context.Background())
		return r.Status + "|" + r.WorkIdentity + "|" + map[bool]string{true: "restart", false: ""}[r.RestartRequired], err
	}
}

func TestPreparationGrantsTheConfiguredWorkIdentityAsTheTrustedHost(t *testing.T) {
	engine := &fakeEngine{}
	_, run := prepare(t, madeConfig(t, embeddedLauncher, map[string]string{hostIdentityKey: "made-local-host-abc-axlr-work"}), engine)
	got, err := run()
	if err != nil || got != "granted|made-local-host-abc-axlr-work|" {
		t.Fatalf("%s %v", got, err)
	}
	issue, verify := engine.calls[0], engine.calls[1]
	if slices.ContainsFunc(issue.env, func(e string) bool { return strings.HasPrefix(e, hostIdentityKey+"=") }) {
		t.Fatal("grant issued under the work identity instead of the trusted host")
	}
	if issue.args["grant_id"] != WorkGrantID || issue.args["grantee_id"] != "made-local-host-abc-axlr-work" {
		t.Fatalf("%v", issue.args)
	}
	for _, action := range issue.args["actions"].([]string) {
		if slices.Contains([]string{"approve_ceremony_guard", "issue_authorization_grant", "revoke_authorization_grant", "publish_ceremony_definition", "validate_ceremony_draft"}, action) {
			t.Fatalf("work grant includes privileged action %s", action)
		}
	}
	if verify.tool != "made_list_ceremony_definitions" || !slices.Contains(verify.env, hostIdentityKey+"=made-local-host-abc-axlr-work") {
		t.Fatalf("readback did not run as the work identity: %+v", verify)
	}
}

func TestPreparationIsANoOpForAnExistingIdenticalGrant(t *testing.T) {
	_, run := prepare(t, madeConfig(t, embeddedLauncher, map[string]string{hostIdentityKey: "axlr-work-0123"}), &fakeEngine{existing: true})
	if got, err := run(); err != nil || got != "ready|axlr-work-0123|" {
		t.Fatalf("%s %v", got, err)
	}
}

func TestPreparationStopsAXLRRunningAsTheTrustedHost(t *testing.T) {
	path := madeConfig(t, embeddedLauncher, map[string]string{})
	_, run := prepare(t, path, &fakeEngine{})
	got, err := run()
	if err != nil || !strings.HasPrefix(got, "granted|axlr-work-") || !strings.HasSuffix(got, "|restart") {
		t.Fatalf("%s %v", got, err)
	}
	configuration, err := storage.LoadMCPConfiguration(path, func(string) string { return "/home/test" })
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(configuration.Registrations[0].Env, func(e string) bool { return strings.HasPrefix(e, hostIdentityKey+"=axlr-work-") }) {
		t.Fatal("work identity was not persisted")
	}
	second, err := run()
	if err != nil || strings.HasSuffix(second, "|restart") || strings.Split(second, "|")[1] != strings.Split(got, "|")[1] {
		t.Fatalf("second run not stable: %s %v", second, err)
	}
}

func TestPreparationLeavesOperatorOwnedSetupsAlone(t *testing.T) {
	for name, path := range map[string]string{
		"explicit host": madeConfig(t, embeddedLauncher, map[string]string{hostIdentityKey: "my-local-host"}),
		"grpc":          madeConfig(t, embeddedLauncher, map[string]string{"MADE_MCP_BACKEND": "grpc"}),
		"other command": madeConfig(t, "made-mcp", map[string]string{}),
	} {
		engine := &fakeEngine{}
		_, run := prepare(t, path, engine)
		got, err := run()
		if err != nil || !(strings.HasPrefix(got, "unsupported|") || strings.HasPrefix(got, "remote|")) || len(engine.calls) != 0 {
			t.Fatalf("%s: %s %v %d calls", name, got, err, len(engine.calls))
		}
	}
}

func TestPreparationReportsAConflictingGrant(t *testing.T) {
	engine := &fakeEngine{refuse: &refusal{Code: "conflict", Message: "changed"}}
	_, run := prepare(t, madeConfig(t, embeddedLauncher, map[string]string{hostIdentityKey: "axlr-work-1"}), engine)
	if got, err := run(); err != nil || !strings.HasPrefix(got, "conflict|") || len(engine.calls) != 1 {
		t.Fatalf("%s %v", got, err)
	}
}

func TestPreparationPublishesCeremoniesThroughAShortLivedGrant(t *testing.T) {
	engine := &fakeEngine{}
	_, run := prepare(t, madeConfig(t, embeddedLauncher, map[string]string{hostIdentityKey: "axlr-work-1"}), engine)
	if _, err := run(); err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, c := range engine.calls {
		order = append(order, c.tool)
		admin := !slices.ContainsFunc(c.env, func(e string) bool { return strings.HasPrefix(e, hostIdentityKey+"=") })
		switch c.tool {
		case "made_publish_ceremony_definition":
			if admin {
				t.Fatal("published as the trusted host instead of the work identity")
			}
		case "made_revoke_authorization_grant":
			if !admin || c.args["reason"] == "" {
				t.Fatalf("revoke not done by the trusted host with a reason: %+v", c.args)
			}
		}
	}
	joined := strings.Join(order, ",")
	if strings.Count(joined, "made_publish_ceremony_definition") != 2 || !strings.HasSuffix(joined, "made_revoke_authorization_grant") {
		t.Fatalf("unexpected sequence: %s", joined)
	}
	engine.calls = nil
	if _, err := run(); err != nil {
		t.Fatal(err)
	}
	for _, c := range engine.calls {
		if c.tool == "made_publish_ceremony_definition" || c.tool == "made_revoke_authorization_grant" {
			t.Fatalf("second preparation republished: %s", c.tool)
		}
	}
}
