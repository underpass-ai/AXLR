package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/domain"
	"github.com/underpass-ai/AXLR/tui/dto"
)

// The skills read bare or wrapped, the catalog optional; what the contract
// does not describe is refused, and so is a requested pin carrying the
// bundle digest the install supplies.
func TestTheSkillsListReadsStrictly(t *testing.T) {
	for raw, want := range map[string][]dto.ExecutionSkillPin{
		`[{"name": "incident_triage", "version": "1.0.0", "catalog": "axlr"}]`:                                                          {{Name: "incident_triage", Version: "1.0.0", Catalog: "axlr"}},
		`{"skills": [{"name": "incident_triage", "version": "1.0.0"}, {"name": "postmortem", "version": "2.10.3", "catalog": "acme"}]}`: {{Name: "incident_triage", Version: "1.0.0"}, {Name: "postmortem", Version: "2.10.3", Catalog: "acme"}},
		`[]`:             {},
		`{"skills": []}`: {},
	} {
		got, err := ParseExecutionSkills([]byte(raw))
		if err != nil || got == nil || len(got) != len(want) {
			t.Fatalf("%s: %+v %v", raw, got, err)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s: pin %d is %+v, want %+v", raw, i, got[i], want[i])
			}
		}
	}
	var many []string
	for i := range domain.MaxSkillPins + 1 {
		many = append(many, fmt.Sprintf(`{"name": "skill_%02d", "version": "1.0.0"}`, i))
	}
	for raw, why := range map[string]string{
		`[{"name": "incident_triage", "version": "1.0.0", "pinned": true}]`:                        "no other field",
		`{"skills": [{"name": "incident_triage", "version": "1.0.0"}], "policy": {}}`:              "no other field",
		`[{"name": "incident_triage", "version": "1.0.0", "bundle_sha256": "` + zeroDigest + `"}]`: "bundle_sha256",
		`[{"name": "incident_triage", "version": "1.0.0"}] []`:                                     "trailing data",
		`{"name": "incident_triage", "version": "1.0.0"}`:                                          "list of pins",
		`"incident_triage"`:                 "list of pins",
		`null`:                              "list of pins",
		`{"skills": null}`:                  "list of pins",
		`[` + strings.Join(many, ",") + `]`: "at most 32",
		`[{"name": "incident_triage", "version": "1.0"}]`:                      "version",
		`[{"name": "incident_triage", "version": "v1.0.0"}]`:                   "version",
		`[{"name": "incident_triage"}]`:                                        "version",
		`[{"name": "Incident", "version": "1.0.0"}]`:                           "name",
		`[{"name": "incident_triage", "version": "1.0.0", "catalog": "ACME"}]`: "catalog",
	} {
		if got, err := ParseExecutionSkills([]byte(raw)); err == nil || !strings.Contains(err.Error(), why) {
			t.Fatalf("%s: want an error naming %q, got %+v %v", raw, why, got, err)
		}
	}
	if _, err := ParseExecutionSkills([]byte(`[` + strings.Repeat(" ", MaxExecutionSkillsBytes) + `]`)); err == nil || !strings.Contains(err.Error(), "16 KiB") {
		t.Fatalf("%v", err)
	}
}

// Resolved pins go to the wire and back unchanged, the wire form is never
// null, and the store's pins are validated on the way in.
func TestTheSkillsWireFormRoundTrips(t *testing.T) {
	skills := domain.ExecutionSkills{Pins: []domain.SkillPin{
		{Catalog: "axlr", Name: "incident_triage", Version: "1.0.0", BundleSHA256: strings.Repeat("a", 64)},
		{Catalog: "acme", Name: "postmortem", Version: "2.10.3", BundleSHA256: strings.Repeat("b", 64)},
	}}
	wire := ExecutionSkills(skills)
	encoded, _ := json.Marshal(wire)
	if string(encoded) != `[{"name":"incident_triage","version":"1.0.0","catalog":"axlr","bundle_sha256":"`+strings.Repeat("a", 64)+`"},{"name":"postmortem","version":"2.10.3","catalog":"acme","bundle_sha256":"`+strings.Repeat("b", 64)+`"}]` {
		t.Fatalf("%s", encoded)
	}
	back, err := DomainExecutionSkills(wire)
	if err != nil || len(back.Pins) != 2 || back.Pins[0] != skills.Pins[0] || back.Pins[1] != skills.Pins[1] {
		t.Fatalf("%+v %v", back, err)
	}
	if empty, _ := json.Marshal(ExecutionSkills(domain.ExecutionSkills{})); string(empty) != `[]` {
		t.Fatalf("no pin is %s, not []", empty)
	}
	if none, err := DomainExecutionSkills([]dto.ExecutionSkillPin{}); err != nil || none.Pins == nil || len(none.Pins) != 0 {
		t.Fatalf("%+v %v", none, err)
	}
	for name, bad := range map[string][]dto.ExecutionSkillPin{
		"no digest":    {{Name: "incident_triage", Version: "1.0.0", Catalog: "axlr"}},
		"no catalog":   {{Name: "incident_triage", Version: "1.0.0", BundleSHA256: strings.Repeat("a", 64)}},
		"pinned twice": {{Name: "incident_triage", Version: "1.0.0", Catalog: "axlr", BundleSHA256: strings.Repeat("a", 64)}, {Name: "incident_triage", Version: "1.0.1", Catalog: "axlr", BundleSHA256: strings.Repeat("a", 64)}},
	} {
		if _, err := DomainExecutionSkills(bad); err == nil {
			t.Fatalf("%s: a stored pin list that does not validate was read", name)
		}
	}
}

// The receipt names the pins with the digest of their compact JSON, the
// same for the same pins, and an empty list for an execution pinned to no
// skill; without pins there is no skills member.
func TestExecutionReceiptDigestsThePinnedSkills(t *testing.T) {
	skills := domain.ExecutionSkills{Pins: []domain.SkillPin{{Catalog: "axlr", Name: "incident_triage", Version: "1.0.0", BundleSHA256: strings.Repeat("c", 64)}}}
	receipt := domain.ExecutionReceipt{Status: domain.ExecutionCompleted, Session: "s", Skills: &skills, Complete: true}
	wire := ExecutionReceipt(receipt)
	compact := `[{"name":"incident_triage","version":"1.0.0","catalog":"axlr","bundle_sha256":"` + strings.Repeat("c", 64) + `"}]`
	sum := sha256.Sum256([]byte(compact))
	if wire.Skills == nil || wire.Skills.SHA256 != hex.EncodeToString(sum[:]) || len(wire.Skills.Pins) != 1 || wire.Skills.Pins[0].BundleSHA256 != strings.Repeat("c", 64) {
		t.Fatalf("skills %+v", wire.Skills)
	}
	again := skills.Clone()
	if other := ExecutionReceipt(domain.ExecutionReceipt{Skills: &again}); other.Skills.SHA256 != wire.Skills.SHA256 {
		t.Fatal("the same pins digest differently")
	}
	changed := skills.Clone()
	changed.Pins[0].Version = "1.0.1"
	if other := ExecutionReceipt(domain.ExecutionReceipt{Skills: &changed}); other.Skills.SHA256 == wire.Skills.SHA256 {
		t.Fatal("another version digests the same")
	}
	encoded, _ := json.Marshal(wire)
	var raw struct {
		Skills map[string]json.RawMessage `json:"skills"`
	}
	if err := json.Unmarshal(encoded, &raw); err != nil || string(raw.Skills["pins"]) != compact || len(raw.Skills) != 2 {
		t.Fatalf("%s: %v", encoded, err)
	}
	none := ExecutionReceipt(domain.ExecutionReceipt{Skills: &domain.ExecutionSkills{}})
	if encoded, _ := json.Marshal(none.Skills); none.Skills == nil || !strings.Contains(string(encoded), `"pins":[]`) {
		t.Fatalf("pinned to no skill: %s", encoded)
	}
	unpinned, _ := json.Marshal(ExecutionReceipt(domain.ExecutionReceipt{}))
	if strings.Contains(string(unpinned), `"skills"`) {
		t.Fatalf("an unpinned receipt names skills: %s", unpinned)
	}
}
