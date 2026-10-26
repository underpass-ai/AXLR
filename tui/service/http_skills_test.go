package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/dto"
)

// installServiceSkill installs one catalog skill version in store, as
// `axlr-tui skills install` would.
func installServiceSkill(t *testing.T, store *storage.CatalogSkillStore, catalog, name, version string) application.InstalledSkill {
	t.Helper()
	content := []byte("# " + name + " " + version + "\n")
	sum := sha256.Sum256(content)
	bundle := sha256.Sum256([]byte(catalog + name + version))
	record := application.InstalledSkill{Catalog: catalog, Name: name, Version: version, Description: "Use for " + name + ".", Files: map[string]string{"SKILL.md": hex.EncodeToString(sum[:])}, BundleSHA256: hex.EncodeToString(bundle[:]), InstalledAt: time.Now().UTC()}
	if _, err := store.Install(context.Background(), record, []application.SkillFile{{Path: "SKILL.md", Content: content}}); err != nil {
		t.Fatal(err)
	}
	return record
}

// CreateSession takes the execution's skills when this service has them
// installed, resolved to their catalog and bundle digest; GetSession shows
// them (null without pins, [] pinned to none) and the receipt names them.
// A pin not installed here, or a list the contract does not describe, is
// refused with the session.
func TestCreateSessionPinsInstalledSkills(t *testing.T) {
	store, err := storage.NewCatalogSkillStore(filepath.Join(t.TempDir(), "skills"))
	if err != nil {
		t.Fatal(err)
	}
	triage := installServiceSkill(t, store, "axlr", "incident_triage", "1.0.0")
	installServiceSkill(t, store, "axlr", "incident_triage", "1.1.0")
	installServiceSkill(t, store, "axlr", "postmortem", "1.0.0")
	installServiceSkill(t, store, "acme", "postmortem", "1.0.0")
	s, ts, client, _ := testServerWith(t, func(_ Config, deps *Dependencies) { deps.Skills = store })
	refused := func(body string) (int, string, string) {
		t.Helper()
		response := apiRequest(t, client, "POST", ts.URL+"/v1/sessions", body, "")
		defer response.Body.Close()
		var failure struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.NewDecoder(response.Body).Decode(&failure)
		return response.StatusCode, failure.Error.Code, failure.Error.Message
	}
	for name, tc := range map[string]struct {
		body, code, message string
	}{
		"not installed":     {`{"model":"test/model","skills":[{"name":"runbook","version":"1.0.0"}]}`, "skills_unavailable", "runbook is not installed"},
		"version not here":  {`{"model":"test/model","skills":[{"name":"incident_triage","version":"2.0.0"}]}`, "skills_unavailable", "installed: 1.0.0, 1.1.0"},
		"two catalogs":      {`{"model":"test/model","skills":[{"name":"postmortem","version":"1.0.0"}]}`, "skills_unavailable", "name the catalog"},
		"digest requested":  {`{"model":"test/model","skills":[{"name":"incident_triage","version":"1.0.0","bundle_sha256":"` + triage.BundleSHA256 + `"}]}`, "invalid_skills", "bundle_sha256"},
		"unknown field":     {`{"model":"test/model","skills":[{"name":"incident_triage","version":"1.0.0","latest":true}]}`, "invalid_skills", "no other field"},
		"not a list":        {`{"model":"test/model","skills":"incident_triage"}`, "invalid_skills", ""},
		"bad version":       {`{"model":"test/model","skills":[{"name":"incident_triage","version":"latest"}]}`, "invalid_skills", "version"},
		"pinned twice":      {`{"model":"test/model","skills":[{"name":"incident_triage","version":"1.0.0"},{"name":"incident_triage","version":"1.1.0"}]}`, "", "pinned twice"},
		"wrapped list form": {`{"model":"test/model","skills":{"skills":[{"name":"runbook","version":"1.0.0"}]}}`, "skills_unavailable", "runbook"},
	} {
		status, code, message := refused(tc.body)
		if status != http.StatusUnprocessableEntity || tc.code != "" && code != tc.code || !strings.Contains(message, tc.message) {
			t.Fatalf("%s: status %d error %q %q", name, status, code, message)
		}
	}
	create := func(body string) string {
		t.Helper()
		response := apiRequest(t, client, "POST", ts.URL+"/v1/sessions", body, "")
		defer response.Body.Close()
		if response.StatusCode != http.StatusCreated {
			t.Fatalf("%s: create status %d", body, response.StatusCode)
		}
		var created struct {
			ID string `json:"id"`
		}
		if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
			t.Fatal(err)
		}
		return created.ID
	}
	shown := func(id string) (json.RawMessage, *dto.ExecutionReceipt) {
		t.Helper()
		response := apiRequest(t, client, "GET", ts.URL+"/v1/sessions/"+id, "", "")
		defer response.Body.Close()
		var session struct {
			Skills  json.RawMessage       `json:"skills"`
			Receipt *dto.ExecutionReceipt `json:"receipt"`
		}
		if err := json.NewDecoder(response.Body).Decode(&session); err != nil {
			t.Fatal(err)
		}
		return session.Skills, session.Receipt
	}
	id := create(`{"model":"test/model","skills":[{"name":"incident_triage","version":"1.0.0"},{"name":"postmortem","version":"1.0.0","catalog":"acme"}]}`)
	skills, _ := shown(id)
	var pins []dto.ExecutionSkillPin
	if err := json.Unmarshal(skills, &pins); err != nil || len(pins) != 2 {
		t.Fatalf("GetSession skills %s: %v", skills, err)
	}
	if pins[0] != (dto.ExecutionSkillPin{Name: "incident_triage", Version: "1.0.0", Catalog: "axlr", BundleSHA256: triage.BundleSHA256}) || pins[1].Catalog != "acme" || len(pins[1].BundleSHA256) != 64 {
		t.Fatalf("pins %+v", pins)
	}
	// The receipt of the session's turn names the pins and their digest.
	response := apiRequest(t, client, "POST", ts.URL+"/v1/sessions/"+id+"/turns", `{"prompt":"triage it","expected_revision":1}`, "turn-key-skills-1234567")
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("turn status %d", response.StatusCode)
	}
	response.Body.Close()
	deadline := time.Now().Add(5 * time.Second)
	for {
		history, _, err := s.events.Read(id, 0)
		if err != nil {
			t.Fatal(err)
		}
		if n := len(history); n > 0 && (history[n-1].Type == "turn.completed" || history[n-1].Type == "operation.failed" || history[n-1].Type == "turn.interrupted") {
			if history[n-1].Type != "turn.completed" {
				t.Fatalf("the turn ended %s", history[n-1].Type)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the turn did not end: %+v", history)
		}
		time.Sleep(10 * time.Millisecond)
	}
	_, receipt := shown(id)
	if receipt == nil || receipt.Skills == nil || len(receipt.Skills.Pins) != 2 || receipt.Skills.Pins[0].BundleSHA256 != triage.BundleSHA256 || len(receipt.Skills.SHA256) != 64 {
		t.Fatalf("receipt %+v", receipt)
	}
	if skills, _ := shown(create(`{"model":"test/model"}`)); string(skills) != "null" {
		t.Fatalf("an unpinned session shows skills %s", skills)
	}
	if skills, _ := shown(create(`{"model":"test/model","skills":[]}`)); string(skills) != "[]" {
		t.Fatalf("a session pinned to no skill shows %s", skills)
	}
	if skills, _ := shown(create(`{"model":"test/model","skills":null}`)); string(skills) != "null" {
		t.Fatalf("skills null shows %s", skills)
	}
	// Without installed skills every pin is unavailable, and no pin is
	// still an execution with no catalog skill.
	s.deps.Skills = nil
	if status, code, _ := refused(`{"model":"test/model","skills":[{"name":"incident_triage","version":"1.0.0"}]}`); status != http.StatusUnprocessableEntity || code != "skills_unavailable" {
		t.Fatalf("pins without installed skills: status %d error %q", status, code)
	}
	if skills, _ := shown(create(`{"model":"test/model","skills":[]}`)); string(skills) != "[]" {
		t.Fatalf("no pin without installed skills shows %s", skills)
	}
}
