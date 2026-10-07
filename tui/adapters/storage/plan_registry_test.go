package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestPlanRegistryRoundTripsAPlan(t *testing.T) {
	path := filepath.Join(t.TempDir(), "axlr", "plans.json")
	registry, err := NewPlanRegistry(path)
	if err != nil {
		t.Fatal(err)
	}
	plan := domain.PlanRecord{ID: "count-words-1a2b3c", Session: "0123456789abcdef0123456789abcdef", Brief: "count words", Workspace: "/w", Planner: "z-ai/glm-5.3-flash", Worker: "local/gemma",
		Status: domain.PlanAwaitingApproval, E2E: domain.CheckCommand{Program: "go", Args: []string{"test", "./..."}}, E2EBaseline: 1, Waves: 2, Interfaces: "WordCount(s) int",
		Tasks: []domain.PlanTask{{ID: "wordcount", Goal: "fix", Scope: []string{"textstat.go"}, Context: []domain.Citation{{Path: "textstat.go", Line: 8, Quote: "strings.Split"}}, UnitCheck: domain.CheckCommand{Program: "go", Args: []string{"test"}}, Wave: 1, Pack: "pack", Status: domain.TaskDone,
			Handback: &domain.TaskHandback{Summary: "done", Notes: []domain.TaskNote{{From: "wordcount", To: "linecount", Text: "use Fields"}}}}},
		Syncs:   []domain.SyncRecord{{Wave: 1, Verdict: "green"}},
		Created: time.Unix(1, 0).UTC(), Updated: time.Unix(2, 0).UTC()}
	if err := registry.Save(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	plan.Status = domain.PlanReady
	if err := registry.Save(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	loaded, err := registry.Load(context.Background())
	if err != nil || len(loaded) != 1 {
		t.Fatalf("load: %v %v", loaded, err)
	}
	got := loaded[0]
	if got.Status != domain.PlanReady || got.Tasks[0].Handback.Notes[0].Text != "use Fields" || got.Tasks[0].Context[0].Line != 8 || got.Syncs[0].Verdict != "green" || got.E2E.Args[1] != "./..." {
		t.Fatalf("round trip: %+v", got)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("registry is not private: %v %v", info, err)
	}
}
