package domain

import (
	"strings"
	"testing"
)

// A team's name opens its members' abouts, project:<team>.<member>, so it
// takes nothing an about refuses and nothing that would read as another
// team's member.
func TestMemoryTeamNameIsOneAboutSegment(t *testing.T) {
	for _, name := range []string{"qa", "mkt-editorial", "equipo_ñ", strings.Repeat("a", 64)} {
		if _, err := NewMemoryTeam(name); err != nil {
			t.Fatalf("%q refused: %v", name, err)
		}
	}
	for _, name := range []string{"", "qa dev", "qa\tdev", "qa\x00", "qa.dev", "qa:dev", "qa/dev", `qa\dev`, strings.Repeat("a", 65), "qa\xff"} {
		if _, err := NewMemoryTeam(name); err == nil {
			t.Fatalf("%q accepted", name)
		}
	}
}

// The team rides the policy: a clone, and so the session's copy, keeps it.
func TestExecutionPolicyKeepsItsMemoryTeam(t *testing.T) {
	policy, err := NewExecutionPolicy(PolicyObserve, []PolicyRule{{Name: "local_read"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, shared := policy.MemoryTeam(); shared {
		t.Fatal("a policy shares its memory with a team it was not given")
	}
	shared, err := policy.WithMemoryTeam("qa")
	if err != nil {
		t.Fatal(err)
	}
	if team, ok := shared.clone().MemoryTeam(); !ok || team != "qa" || len(shared.Allow()) != 1 || shared.Level() != PolicyObserve {
		t.Fatalf("%+v", shared)
	}
	if _, ok := policy.MemoryTeam(); ok {
		t.Fatal("WithMemoryTeam changed the policy it was called on")
	}
	if _, err := policy.WithMemoryTeam("qa.dev"); err == nil {
		t.Fatal("a bad team name accepted")
	}
}
