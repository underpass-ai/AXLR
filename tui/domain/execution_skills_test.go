package domain

import (
	"fmt"
	"strings"
	"testing"

	axlr "github.com/underpass-ai/AXLR/domain"
)

// skillDigest is a well-formed bundle digest made of one repeated hex
// digit.
func skillDigest(digit string) string { return strings.Repeat(digit, 64) }

func skillPin(catalog, name, version string) SkillPin {
	return SkillPin{Catalog: catalog, Name: name, Version: version, BundleSHA256: skillDigest("a")}
}

// A pin names a configured catalog, a skill name, a major.minor.patch
// version and the installed bundle's digest; an execution pins each
// catalog's skill once and at most MaxSkillPins skills, and no pin at all
// is an execution with no catalog skill.
func TestExecutionSkillsValidate(t *testing.T) {
	good := skillPin("axlr", "incident_triage", "1.0.0")
	for name, tc := range map[string]struct {
		skills ExecutionSkills
		want   string
	}{
		"empty":                {ExecutionSkills{}, ""},
		"empty list":           {ExecutionSkills{Pins: []SkillPin{}}, ""},
		"one pin":              {ExecutionSkills{Pins: []SkillPin{good}}, ""},
		"same name, two cats":  {ExecutionSkills{Pins: []SkillPin{good, skillPin("acme", "incident_triage", "2.0.0")}}, ""},
		"no catalog":           {ExecutionSkills{Pins: []SkillPin{skillPin("", "incident_triage", "1.0.0")}}, "catalog"},
		"catalog uppercase":    {ExecutionSkills{Pins: []SkillPin{skillPin("Axlr", "incident_triage", "1.0.0")}}, "catalog"},
		"catalog too long":     {ExecutionSkills{Pins: []SkillPin{skillPin("a"+strings.Repeat("b", 32), "incident_triage", "1.0.0")}}, "catalog"},
		"name one letter":      {ExecutionSkills{Pins: []SkillPin{skillPin("axlr", "i", "1.0.0")}}, "name"},
		"name with dash":       {ExecutionSkills{Pins: []SkillPin{skillPin("axlr", "incident-triage", "1.0.0")}}, "name"},
		"name from a digit":    {ExecutionSkills{Pins: []SkillPin{skillPin("axlr", "1triage", "1.0.0")}}, "name"},
		"version two parts":    {ExecutionSkills{Pins: []SkillPin{skillPin("axlr", "incident_triage", "1.0")}}, "version"},
		"version leading zero": {ExecutionSkills{Pins: []SkillPin{skillPin("axlr", "incident_triage", "01.0.0")}}, "version"},
		"version prerelease":   {ExecutionSkills{Pins: []SkillPin{skillPin("axlr", "incident_triage", "1.0.0-rc1")}}, "version"},
		"digest missing":       {ExecutionSkills{Pins: []SkillPin{{Catalog: "axlr", Name: "incident_triage", Version: "1.0.0"}}}, "digest"},
		"digest uppercase":     {ExecutionSkills{Pins: []SkillPin{{Catalog: "axlr", Name: "incident_triage", Version: "1.0.0", BundleSHA256: skillDigest("A")}}}, "digest"},
		"digest short":         {ExecutionSkills{Pins: []SkillPin{{Catalog: "axlr", Name: "incident_triage", Version: "1.0.0", BundleSHA256: strings.Repeat("a", 63)}}}, "digest"},
		"pinned twice":         {ExecutionSkills{Pins: []SkillPin{good, skillPin("axlr", "incident_triage", "2.0.0")}}, "pinned twice"},
	} {
		err := tc.skills.Validate()
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("%s: %v", name, err)
		case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
			t.Errorf("%s: want an error naming %q, got %v", name, tc.want, err)
		}
	}
	var pins []SkillPin
	for i := range MaxSkillPins {
		pins = append(pins, skillPin("axlr", fmt.Sprintf("skill_%02d", i), "1.0.0"))
	}
	if err := (ExecutionSkills{Pins: pins}).Validate(); err != nil {
		t.Fatalf("%d pins: %v", MaxSkillPins, err)
	}
	pins = append(pins, skillPin("axlr", "one_too_many", "1.0.0"))
	if err := (ExecutionSkills{Pins: pins}).Validate(); err == nil || !strings.Contains(err.Error(), "at most 32") {
		t.Fatalf("%d pins: %v", len(pins), err)
	}
}

// Pin finds a catalog's skill; Clone does not share the pins.
func TestExecutionSkillsPinAndClone(t *testing.T) {
	skills := ExecutionSkills{Pins: []SkillPin{skillPin("axlr", "incident_triage", "1.0.0"), skillPin("acme", "incident_triage", "2.0.0")}}
	if pin, has := skills.Pin("acme", "incident_triage"); !has || pin.Version != "2.0.0" {
		t.Fatalf("%+v %v", pin, has)
	}
	if _, has := skills.Pin("other", "incident_triage"); has {
		t.Fatal("a pin found in a catalog that does not have it")
	}
	clone := skills.Clone()
	clone.Pins[0].Version = "9.9.9"
	if skills.Pins[0].Version != "1.0.0" {
		t.Fatal("the clone shares its pins")
	}
}

// A session pins its skills from SetSkills on: an empty pin list stays a
// pin (no catalog skill), unlike no pins at all (the newest installed);
// Skills returns a copy; an invalid list is refused and the earlier pins
// kept; ClearSkills lifts them; a restored session keeps them.
func TestSessionPinsItsSkills(t *testing.T) {
	s := session(t)
	if _, has := s.Skills(); has {
		t.Fatal("a new session is pinned")
	}
	must(t, s.SetSkills(ExecutionSkills{}))
	skills, has := s.Skills()
	if !has || len(skills.Pins) != 0 {
		t.Fatalf("an empty pin list did not stay a pin: %+v %v", skills, has)
	}
	pins := ExecutionSkills{Pins: []SkillPin{skillPin("axlr", "incident_triage", "1.0.0")}}
	must(t, s.SetSkills(pins))
	pins.Pins[0].Version = "2.0.0"
	got, has := s.Skills()
	if !has || len(got.Pins) != 1 || got.Pins[0].Version != "1.0.0" {
		t.Fatalf("SetSkills kept the caller's slice: %+v", got)
	}
	got.Pins[0].Version = "3.0.0"
	if again, _ := s.Skills(); again.Pins[0].Version != "1.0.0" {
		t.Fatal("Skills returned the session's own slice")
	}
	if err := s.SetSkills(ExecutionSkills{Pins: []SkillPin{skillPin("axlr", "incident_triage", "latest")}}); err == nil {
		t.Fatal("an invalid pin was set")
	}
	if kept, _ := s.Skills(); len(kept.Pins) != 1 || kept.Pins[0].Version != "1.0.0" {
		t.Fatalf("a refused pin list changed the pins: %+v", kept)
	}
	state := s.Export()
	if state.Skills == nil || len(state.Skills.Pins) != 1 {
		t.Fatalf("export %+v", state.Skills)
	}
	restored, err := RestoreSession(state)
	must(t, err)
	if kept, has := restored.Skills(); !has || len(kept.Pins) != 1 || kept.Pins[0] != skillPin("axlr", "incident_triage", "1.0.0") {
		t.Fatalf("restored %+v %v", kept, has)
	}
	state.Skills = &ExecutionSkills{Pins: []SkillPin{skillPin("axlr", "incident_triage", "1.0.0"), skillPin("axlr", "incident_triage", "1.0.1")}}
	if _, err := RestoreSession(state); err == nil {
		t.Fatal("a snapshot pinning one skill twice was restored")
	}
	s.ClearSkills()
	if _, has := s.Skills(); has {
		t.Fatal("cleared pins stay")
	}
}

// The receipt carries the pins of the last session the execution ran in,
// an empty list for an execution pinned to no skill, and none without
// pins.
func TestExecutionReceiptNamesThePinnedSkills(t *testing.T) {
	first := session(t)
	must(t, first.SetSkills(ExecutionSkills{Pins: []SkillPin{skillPin("axlr", "incident_triage", "1.0.0")}}))
	last, err := NewSession("fedcba9876543210fedcba9876543210", first.Export().Workspace, "test/model")
	must(t, err)
	must(t, last.SetSkills(ExecutionSkills{Pins: []SkillPin{skillPin("axlr", "incident_triage", "1.1.0")}}))
	for _, s := range []*Session{&first, &last} {
		must(t, s.BeginTurn("hello", catalog(t)))
		must(t, s.CompleteAssistant(axlr.CompletionResult{Message: axlr.Message{Role: axlr.RoleAssistant, Content: "done"}}))
	}
	receipt := NewExecutionReceipt(NewExecutionResult(last, nil), []ReceiptSession{{Session: first}, {Session: last}})
	if receipt.Skills == nil || len(receipt.Skills.Pins) != 1 || receipt.Skills.Pins[0].Version != "1.1.0" {
		t.Fatalf("receipt skills %+v", receipt.Skills)
	}
	none := session(t)
	must(t, none.SetSkills(ExecutionSkills{}))
	must(t, none.BeginTurn("hello", catalog(t)))
	must(t, none.CompleteAssistant(axlr.CompletionResult{Message: axlr.Message{Role: axlr.RoleAssistant, Content: "done"}}))
	if receipt := NewExecutionReceipt(NewExecutionResult(none, nil), []ReceiptSession{{Session: none}}); receipt.Skills == nil || len(receipt.Skills.Pins) != 0 {
		t.Fatalf("pinned to no skill: %+v", receipt.Skills)
	}
	unpinned := session(t)
	must(t, unpinned.BeginTurn("hello", catalog(t)))
	must(t, unpinned.CompleteAssistant(axlr.CompletionResult{Message: axlr.Message{Role: axlr.RoleAssistant, Content: "done"}}))
	if receipt := NewExecutionReceipt(NewExecutionResult(unpinned, nil), []ReceiptSession{{Session: unpinned}}); receipt.Skills != nil {
		t.Fatalf("an unpinned execution's receipt names skills: %+v", receipt.Skills)
	}
}
