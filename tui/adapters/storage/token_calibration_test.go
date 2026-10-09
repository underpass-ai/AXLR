package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestTokenCalibrationLearnsAndPersistsEachModel(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "axlr", "bytes-per-token.json")
	first, err := NewTokenCalibration(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < domain.CalibrationSamples-1; i++ {
		if err := first.Observe(ctx, "z-ai/glm-5.3-flash", 42000, 10000); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := first.BytesPerToken("z-ai/glm-5.3-flash"); ok {
		t.Fatal("applied before enough samples")
	}
	if err := first.Observe(ctx, "z-ai/glm-5.3-flash", 42000, 10000); err != nil {
		t.Fatal(err)
	}
	if got, ok := first.BytesPerToken("z-ai/glm-5.3-flash"); !ok || got != 420 {
		t.Fatalf("ratio = %d %v", got, ok)
	}
	// A request too small to measure writes nothing.
	before, _ := os.ReadFile(path)
	if err := first.Observe(ctx, "z-ai/glm-5.3-flash", 900, 300); err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Fatal("a small prompt changed the file")
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("file mode = %v, %v", info.Mode(), err)
		}
	}
	// A later console starts from what this one measured.
	second, err := NewTokenCalibration(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := second.BytesPerToken("z-ai/glm-5.3-flash"); !ok || got != 420 {
		t.Fatalf("reloaded ratio = %d %v", got, ok)
	}
	if _, ok := second.BytesPerToken("anthropic/claude-haiku-5.5"); ok {
		t.Fatal("an unmeasured model has a ratio")
	}
}

// The ratio a console applies does not move under a running session: the
// projection's ceiling stays put, and so does the provider's prompt cache.
func TestTokenCalibrationKeepsTheAppliedRatioForTheLaunch(t *testing.T) {
	ctx := context.Background()
	calibration, err := NewTokenCalibration(filepath.Join(t.TempDir(), "bytes-per-token.json"))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < domain.CalibrationSamples; i++ {
		_ = calibration.Observe(ctx, "z-ai/glm-5.3-flash", 42000, 10000)
	}
	applied, _ := calibration.BytesPerToken("z-ai/glm-5.3-flash")
	for i := 0; i < 20; i++ {
		_ = calibration.Observe(ctx, "z-ai/glm-5.3-flash", 30000, 10000)
	}
	if got, _ := calibration.BytesPerToken("z-ai/glm-5.3-flash"); got != applied {
		t.Fatalf("applied ratio moved from %d to %d", applied, got)
	}
	file, err := calibration.read()
	if err != nil || file.Models["z-ai/glm-5.3-flash"].Samples != domain.CalibrationSamples+20 || file.Models["z-ai/glm-5.3-flash"].BytesPerToken >= 4.2 {
		t.Fatalf("stored = %+v, %v", file.Models, err)
	}
}

func TestTokenCalibrationBoundsTheFileAndSurvivesDamage(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "bytes-per-token.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	calibration, err := NewTokenCalibration(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := calibration.BytesPerToken("z-ai/glm-5.3-flash"); ok {
		t.Fatal("a damaged file gave a ratio")
	}
	clock := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	calibration.now = func() time.Time { clock = clock.Add(time.Minute); return clock }
	for i := 0; i <= maxCalibratedModels; i++ {
		if err := calibration.Observe(ctx, root.ModelID(fmt.Sprintf("vendor/model-%02d", i)), 30000, 10000); err != nil {
			t.Fatal(err)
		}
	}
	data, _ := os.ReadFile(path)
	var file tokenCalibrationFile
	if err := json.Unmarshal(data, &file); err != nil || len(file.Models) != maxCalibratedModels {
		t.Fatalf("models kept = %d, %v", len(file.Models), err)
	}
	if _, kept := file.Models["vendor/model-00"]; kept {
		t.Fatal("the oldest model was kept")
	}
	if _, err := NewTokenCalibration("relative.json"); err == nil {
		t.Fatal("a relative path was accepted")
	}
}
