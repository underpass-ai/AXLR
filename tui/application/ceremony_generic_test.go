package application

import (
	"context"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/domain"
)

type definitionsStub struct {
	definition CeremonyDefinition
	listed     []CeremonyDefinitionSummary
}

func (d definitionsStub) Describe(context.Context, string, string) (CeremonyDefinition, error) {
	return d.definition, nil
}
func (d definitionsStub) List(context.Context) ([]CeremonyDefinitionSummary, error) {
	return d.listed, nil
}

// genericEngine is a fake engine for a two-step definition: step one in
// START, trigger "started" to WORK with step two, trigger "done" to DONE.
type genericEngine struct {
	fakeEngine
	stateOf string
}

func (g *genericEngine) Inspect(context.Context, string) (CeremonyView, error) {
	g.calls = append(g.calls, "inspect")
	switch g.stateOf {
	case "", "START":
		g.stateOf = "START"
		if g.completedSteps("one") {
			return CeremonyView{State: "START", Enabled: []string{"started"}}, nil
		}
		return CeremonyView{State: "START", Claimable: []string{"one"}}, nil
	case "WORK":
		if g.completedSteps("two") {
			return CeremonyView{State: "WORK", Enabled: []string{"done"}}, nil
		}
		return CeremonyView{State: "WORK", Claimable: []string{"two"}}, nil
	}
	return CeremonyView{State: g.stateOf}, nil
}

func (g *genericEngine) completedSteps(step string) bool {
	for _, call := range g.calls {
		if call == "complete "+step {
			return true
		}
	}
	return false
}

func (g *genericEngine) Transition(_ context.Context, _, trigger string) (string, error) {
	g.calls = append(g.calls, "transition "+trigger)
	g.stateOf = map[string]string{"started": "WORK", "done": "DONE"}[trigger]
	return g.stateOf, nil
}

// /ceremony runs any published definition: the brief fills its inputs,
// each step's prompt is the instruction, the fields handed back are the
// output, and MADE's enabled trigger decides the transition.
func TestGenericCeremonyRunsAPublishedDefinition(t *testing.T) {
	engine := &genericEngine{}
	memory := &fakeMemory{}
	d := CeremonyDriver{Engine: engine, Checks: &fakeChecks{}, Memory: memory, Definitions: definitionsStub{definition: CeremonyDefinition{Name: "team_review", Version: "1.0", Inputs: []string{"brief", "workspace"}, Initial: "START", Terminal: []string{"DONE"},
		Steps: []DefinitionStep{{ID: "one", State: "START"}, {ID: "two", State: "WORK"}}, Prompts: map[string]string{"one": "Read the change and list its risks.", "two": "Write the review."}}}}
	s := turnSession(t)
	if err := s.ChooseCeremony("team_review", "1.0"); err != nil {
		t.Fatal(err)
	}
	if err := d.Begin(context.Background(), &s, "revisa el cambio"); err != nil {
		t.Fatal(err)
	}
	run, live := s.Ceremony()
	if !live || run.Definition != "team_review" || run.Step != "one" || run.Generic == nil || run.Generic.Prompts["two"] != "Write the review." || run.Fence != "fence-one" {
		t.Fatalf("run %+v", run)
	}
	if !strings.Contains(strings.Join(engine.calls, ";"), "start team_review 1.0 about=ws:"+string(s.ID())) {
		t.Fatalf("calls %v", engine.calls)
	}
	if note := CurrentStepNote(run, true); !strings.Contains(note, "Read the change and list its risks.") || !strings.Contains(note, "axlr_step_done") {
		t.Fatalf("note %q", note)
	}
	if err := s.BeginTurn("revisa el cambio", turnTools()); err != nil {
		t.Fatal(err)
	}
	result, err := d.StepDone(context.Background(), s, mustObject(t, `{"risks":["none"],"why":"hand back"}`))
	if err != nil || !result.Accepted || result.Run == nil || result.Run.Step != "two" {
		t.Fatalf("%+v %v", result, err)
	}
	if engine.completed[0]["risks"] == nil || engine.completed[0]["why"] != nil {
		t.Fatalf("completed %+v", engine.completed)
	}
	if !strings.Contains(strings.Join(engine.calls, ";"), "complete one;inspect;transition started;inspect;claim two") {
		t.Fatalf("calls %v", engine.calls)
	}
	if err := s.SetCeremony(*result.Run); err != nil {
		t.Fatal(err)
	}
	result, err = d.StepDone(context.Background(), s, mustObject(t, `{"review":"fine"}`))
	if err != nil || !result.Accepted || result.Run != nil {
		t.Fatalf("%+v %v", result, err)
	}
	report := result.Outcome.Content
	if !strings.Contains(string(report), `"ceremony":"DONE"`) || !strings.Contains(string(report), "recorded in") {
		t.Fatalf("report %s", report)
	}
}

func TestVersionLessAndLatestVersion(t *testing.T) {
	if !VersionLess("1.10", "2.0") || VersionLess("2.0", "1.10") || !VersionLess("1.9", "1.10") || VersionLess("1.0", "1.0") {
		t.Fatal("version order")
	}
	listed := []CeremonyDefinitionSummary{{Name: "a", Version: "1.0"}, {Name: "a", Version: "2.0"}, {Name: "a", Version: "1.10"}, {Name: "b", Version: "3.0"}}
	if LatestVersion(listed, "a") != "2.0" || LatestVersion(listed, "c") != "" {
		t.Fatal("latest")
	}
}

// Choosing a ceremony switches to ceremony mode; another mode forgets it.
func TestChosenCeremonyFollowsTheMode(t *testing.T) {
	s := turnSession(t)
	if err := s.ChooseCeremony("x", ""); err == nil {
		t.Fatal("a version is required")
	}
	if err := s.ChooseCeremony("x", "1.0"); err != nil {
		t.Fatal(err)
	}
	if s.Mode() != domain.ModeCeremony || !s.Mode().StartsCeremony() || s.ChosenCeremony() != (domain.CeremonyChoice{Definition: "x", Version: "1.0"}) {
		t.Fatalf("%s %+v", s.Mode(), s.ChosenCeremony())
	}
	if err := s.SetMode(domain.ModeNormal); err != nil {
		t.Fatal(err)
	}
	if s.ChosenCeremony() != (domain.CeremonyChoice{}) {
		t.Fatal("the choice survived the mode")
	}
}
