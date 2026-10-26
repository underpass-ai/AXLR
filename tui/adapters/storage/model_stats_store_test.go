package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestModelStatsStoreKeepsWhatRequestsSaidAcrossInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "axlr", "model-stats.json")
	store, err := NewModelStatsStore(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	for i := 0; i < domain.ModelHistorySamples+5; i++ {
		if err := store.ObserveModel(ctx, "anthropic/claude-haiku-5.5", domain.ModelObservation{At: now.Add(time.Duration(i) * time.Second), Provider: "Anthropic", PromptTokens: 1000, CompletionTokens: 50, Cost: 0.001, CostKnown: true, FirstByte: 400 * time.Millisecond, Total: 2 * time.Second}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.ObserveModel(ctx, "local", domain.ModelObservation{At: now, Provider: "local", Failed: true}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("the stats file is not private: %v %v", info, err)
	}
	other, _ := NewModelStatsStore(path)
	stats, err := other.ModelStats(ctx, []root.ModelID{"anthropic/claude-haiku-5.5", "local", "never"}, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	haiku := stats["anthropic/claude-haiku-5.5"]
	if haiku.Requests != domain.ModelHistorySamples || haiku.CostRequests != domain.ModelHistorySamples || haiku.Total.Average() != 2*time.Second {
		t.Fatalf("haiku stats = %+v", haiku)
	}
	if perToken, ok := haiku.CostPerToken(); !ok || perToken <= 0 {
		t.Fatalf("cost per token = %v %v", perToken, ok)
	}
	if local := stats["local"]; local.Failures != 1 || local.TrailingFailures != 1 {
		t.Fatalf("local stats = %+v", local)
	}
	if never := stats["never"]; never.Requests != 0 {
		t.Fatalf("a model never observed has stats %+v", never)
	}
}

func TestModelStatsStoreBoundsItsModelsAndSurvivesDamage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model-stats.json")
	store, _ := NewModelStatsStore(path)
	ctx := context.Background()
	now := time.Now().UTC()
	for i := 0; i < maxModelStatsModels+4; i++ {
		if err := store.ObserveModel(ctx, root.ModelID(fmt.Sprintf("model-%02d", i)), domain.ModelObservation{At: now.Add(time.Duration(i) * time.Second), Provider: "p", Total: time.Second}); err != nil {
			t.Fatal(err)
		}
	}
	stats, _ := store.ModelStats(ctx, []root.ModelID{"model-00", root.ModelID(fmt.Sprintf("model-%02d", maxModelStatsModels+3))}, now.Add(time.Hour))
	if stats["model-00"].Requests != 0 || stats[root.ModelID(fmt.Sprintf("model-%02d", maxModelStatsModels+3))].Requests != 1 {
		t.Fatalf("the oldest model was kept or the newest lost: %+v", stats)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	if stats, err := store.ModelStats(ctx, []root.ModelID{"model-10"}, now); err != nil || len(stats) != 0 {
		t.Fatalf("a damaged file read as %+v, %v", stats, err)
	}
	if err := store.ObserveModel(ctx, "model-10", domain.ModelObservation{At: now, Provider: "p"}); err != nil {
		t.Fatalf("a damaged file refused a new observation: %v", err)
	}
}
