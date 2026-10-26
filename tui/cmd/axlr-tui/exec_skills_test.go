package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/application"
)

// installCLISkill installs one catalog skill version where the console
// reads them, <XDG_STATE_HOME>/axlr/skills, as `axlr-tui skills install`
// would.
func installCLISkill(t *testing.T, env map[string]string, catalog, name, version string) application.InstalledSkill {
	t.Helper()
	store, err := storage.NewCatalogSkillStore(filepath.Join(env["XDG_STATE_HOME"], "axlr", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("# " + name + " " + version + "\nStart from the logs.\n")
	sum := sha256.Sum256(content)
	bundle := sha256.Sum256([]byte(catalog + "/" + name + "@" + version))
	record := application.InstalledSkill{Catalog: catalog, Name: name, Version: version, Description: "Triage a production incident (" + version + ").", Tools: []string{"local_read"}, Files: map[string]string{"SKILL.md": hex.EncodeToString(sum[:])}, BundleSHA256: hex.EncodeToString(bundle[:]), InstalledAt: time.Now().UTC()}
	if _, err := store.Install(context.Background(), record, []application.SkillFile{{Path: "SKILL.md", Content: content}}); err != nil {
		t.Fatal(err)
	}
	return record
}

// recordingModel answers each request with the next chunk, as
// scriptedModel does, and keeps every request body.
func recordingModel(t *testing.T, chunks ...string) *[]string {
	t.Helper()
	previous := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = previous })
	var bodies []string
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		chunk := chunks[min(len(bodies), len(chunks)-1)]
		bodies = append(bodies, string(body))
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("data: " + chunk + "\n\ndata: [DONE]\n\n")), Header: make(http.Header)}, nil
	})
	return &bodies
}

// skillCall asks axlr_skill, under call id, for a catalog skill's SKILL.md.
func skillCall(id, catalog, skill string) string {
	arguments, _ := json.Marshal(map[string]any{"catalog": catalog, "skill": skill})
	call, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": id, "type": "function", "function": map[string]any{"name": "axlr_skill", "arguments": string(arguments)}}}}, "finish_reason": "tool_calls"}}})
	return string(call)
}

// A pin that is not installed ends the run before its first request, and
// so does a skills file the contract does not describe; --skills goes with
// --exec or --decide.
func TestExecRefusesSkillsItCannotServe(t *testing.T) {
	env := cliEnv(t)
	calls := scriptedModel(t, answerChunk)
	installCLISkill(t, env, "axlr", "incident_triage", "1.0.0")
	for name, tc := range map[string]struct {
		skills, want string
	}{
		"not installed":    {`[{"name": "runbook", "version": "1.0.0"}]`, "the execution's skills are unavailable here: runbook is not installed"},
		"another version":  {`{"skills": [{"name": "incident_triage", "version": "2.0.0"}]}`, "the execution's skills are unavailable here: incident_triage 2.0.0 is not installed (installed: 1.0.0)"},
		"another catalog":  {`[{"name": "incident_triage", "version": "1.0.0", "catalog": "acme"}]`, "the execution's skills are unavailable here"},
		"unknown field":    {`[{"name": "incident_triage", "version": "1.0.0", "latest": true}]`, "--skills"},
		"digest requested": {`[{"name": "incident_triage", "version": "1.0.0", "bundle_sha256": "` + strings.Repeat("a", 64) + `"}]`, "bundle_sha256"},
		"not a list":       {`{"name": "incident_triage", "version": "1.0.0"}`, "--skills"},
		"not json":         {`incident_triage@1.0.0`, "--skills"},
	} {
		code, _, stderr := execRun(t, env, "--root", t.TempDir(), "--model", "test/model", "--exec", "triage it", "--skills", writePolicy(t, tc.skills))
		if code != execFailed || *calls != 0 || !strings.Contains(stderr, tc.want) {
			t.Fatalf("%s: code=%d calls=%d stderr=%s", name, code, *calls, stderr)
		}
	}
	code, _, stderr := execRun(t, env, "--root", t.TempDir(), "--model", "test/model", "--exec", "triage it", "--skills", filepath.Join(t.TempDir(), "missing.json"))
	if code != execFailed || *calls != 0 || !strings.Contains(stderr, "--skills") {
		t.Fatalf("missing file: code=%d stderr=%s", code, stderr)
	}
	code, _, stderr = execRun(t, env, "--root", t.TempDir(), "--model", "test/model", "--skills", writePolicy(t, `[]`))
	if code == 0 || *calls != 0 || !strings.Contains(stderr, "--skills and --correlation go with --exec or --decide") {
		t.Fatalf("--skills without --exec: code=%d stderr=%s", code, stderr)
	}
}

// An execution pinned to an older version lists that version in the skill
// index and reads it through axlr_skill while a newer one is installed;
// the receipt names the pin; the session keeps it for the next run, which
// ends before its first request once the pinned version is removed.
func TestExecReadsThePinnedSkillVersion(t *testing.T) {
	env := cliEnv(t)
	pinned := installCLISkill(t, env, "axlr", "incident_triage", "1.0.0")
	installCLISkill(t, env, "axlr", "incident_triage", "1.1.0")
	installCLISkill(t, env, "acme", "postmortem", "1.0.0")
	bodies := recordingModel(t, skillCall("skill-1", "axlr", "incident_triage"), answerChunk)
	workspace := t.TempDir()
	skills := writePolicy(t, `{"skills": [{"name": "incident_triage", "version": "1.0.0"}]}`)
	code, stdout, stderr := execRun(t, env, "--root", workspace, "--model", "test/model", "--exec", "triage it", "--skills", skills, "--json")
	if code != execCompleted || len(*bodies) != 2 || !strings.Contains(stderr, "skills pinned: axlr/incident_triage@1.0.0") {
		t.Fatalf("code=%d requests=%d stderr=%s", code, len(*bodies), stderr)
	}
	first, second := (*bodies)[0], (*bodies)[1]
	if !strings.Contains(first, "catalog axlr: Open these with axlr_skill and catalog axlr instead of plugin. Skills: incident_triage (version 1.0.0:") || strings.Contains(first, "version 1.1.0") || strings.Contains(first, "catalog acme") {
		t.Fatalf("the skill index is not the pinned one: %s", first)
	}
	if !strings.Contains(second, "# incident_triage 1.0.0") || strings.Contains(second, "# incident_triage 1.1.0") {
		t.Fatalf("axlr_skill did not read the pinned version: %s", second)
	}
	lines := execLines(t, stdout)
	receipt := lines[len(lines)-2].Receipt
	if receipt == nil || receipt.Skills == nil || len(receipt.Skills.Pins) != 1 || receipt.Skills.Pins[0].Catalog != "axlr" || receipt.Skills.Pins[0].Version != "1.0.0" || receipt.Skills.Pins[0].BundleSHA256 != pinned.BundleSHA256 || len(receipt.Skills.SHA256) != 64 {
		t.Fatalf("receipt %+v", receipt)
	}
	var skillCalls int
	for _, call := range receipt.Calls {
		if call.Tool == "axlr_skill" && call.Status == "completed" {
			skillCalls++
		}
	}
	if skillCalls != 1 {
		t.Fatalf("the receipt's calls %+v", receipt.Calls)
	}
	result := execResult(t, stdout)
	// The session keeps the pins: the next run reads the same version.
	bodies = recordingModel(t, answerChunk)
	code, stdout, stderr = execRun(t, env, "--root", workspace, "--session", result.Session, "--exec", "again", "--json")
	if code != execCompleted || !strings.Contains(stderr, "skills pinned: axlr/incident_triage@1.0.0") || !strings.Contains((*bodies)[0], "incident_triage (version 1.0.0:") {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if again := execLines(t, stdout); again[len(again)-2].Receipt.Skills.SHA256 != receipt.Skills.SHA256 {
		t.Fatalf("the pins changed: %+v", again[len(again)-2].Receipt.Skills)
	}
	// The pinned version removed, the run ends before its first request.
	store, err := storage.NewCatalogSkillStore(filepath.Join(env["XDG_STATE_HOME"], "axlr", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Remove(context.Background(), "axlr", "incident_triage", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	bodies = recordingModel(t, answerChunk)
	code, _, stderr = execRun(t, env, "--root", workspace, "--session", result.Session, "--exec", "once more")
	if code != execFailed || len(*bodies) != 0 || !strings.Contains(stderr, "the execution's skills are unavailable here: incident_triage 1.0.0 of catalog axlr is no longer installed") {
		t.Fatalf("code=%d requests=%d stderr=%s", code, len(*bodies), stderr)
	}
	// A new --skills replaces the pins.
	bodies = recordingModel(t, answerChunk)
	code, _, stderr = execRun(t, env, "--root", workspace, "--session", result.Session, "--exec", "with the newer one", "--skills", writePolicy(t, `[{"name": "incident_triage", "version": "1.1.0", "catalog": "axlr"}]`))
	if code != execCompleted || !strings.Contains(stderr, "skills pinned: axlr/incident_triage@1.1.0") || !strings.Contains((*bodies)[0], "incident_triage (version 1.1.0:") {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
}

// Without --skills an execution reads every installed catalog skill at its
// newest installed version; pinned to none, it reads none.
func TestExecWithoutSkillsReadsTheNewestInstalled(t *testing.T) {
	env := cliEnv(t)
	installCLISkill(t, env, "axlr", "incident_triage", "1.0.0")
	installCLISkill(t, env, "axlr", "incident_triage", "1.1.0")
	bodies := recordingModel(t, answerChunk)
	code, _, stderr := execRun(t, env, "--root", t.TempDir(), "--model", "test/model", "--exec", "triage it")
	if code != execCompleted || strings.Contains(stderr, "skills pinned") || !strings.Contains((*bodies)[0], "incident_triage (version 1.1.0:") {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	bodies = recordingModel(t, answerChunk)
	code, _, stderr = execRun(t, env, "--root", t.TempDir(), "--model", "test/model", "--exec", "triage it", "--skills", writePolicy(t, `[]`))
	if code != execCompleted || !strings.Contains(stderr, "skills pinned: none (no catalog skill)") || strings.Contains((*bodies)[0], "catalog axlr:") {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
}
