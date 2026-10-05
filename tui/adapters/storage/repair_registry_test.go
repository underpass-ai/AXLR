package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestRepairRegistryUpsertsAndKeepsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "axlr", "repairs.json")
	registry, err := NewRepairRegistry(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewRepairRegistry("relative/repairs.json"); err == nil {
		t.Fatal("relative path accepted")
	}
	ctx := context.Background()
	records, err := registry.Load(ctx)
	if err != nil || len(records) != 0 {
		t.Fatalf("empty registry: %v %v", records, err)
	}
	now := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	record := domain.RepairRecord{ID: "20261005-1200-edit-fails", Signature: "abc", Repository: "o/r", Parent: "0123456789abcdef0123456789abcdef", Status: domain.RepairRunning, Attempt: 1, Created: now, Updated: now, RunToken: "t1"}
	if err := registry.Save(ctx, record); err != nil {
		t.Fatal(err)
	}
	record.Status, record.PullRequest, record.URL, record.Notice = domain.RepairCompleted, 7, "https://example.test/pr/7", "merged"
	if err := registry.Save(ctx, record); err != nil {
		t.Fatal(err)
	}
	other := record
	other.ID, other.Parent = "other", "fedcba9876543210fedcba9876543210"
	if err := registry.Save(ctx, other); err != nil {
		t.Fatal(err)
	}
	records, err = registry.Load(ctx)
	if err != nil || len(records) != 2 || records[0].Status != domain.RepairCompleted || records[0].PullRequest != 7 || records[0].Notice != "merged" || !records[0].Created.Equal(now) {
		t.Fatalf("records: %+v %v", records, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("registry must be private: %v %v", info, err)
	}
	data, _ := os.ReadFile(path)
	data = []byte(strings.Replace(string(data), `"repairs": [`, `"future": true, "repairs": [`, 1))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if records, err = registry.Load(ctx); err != nil || len(records) != 2 {
		t.Fatalf("unknown fields must not hide the records: %v %v", records, err)
	}
	if err := registry.Save(ctx, domain.RepairRecord{ID: "bad"}); err == nil {
		t.Fatal("invalid record saved")
	}
	if err := os.WriteFile(path, []byte(`{"version":9}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Load(ctx); err == nil {
		t.Fatal("wrong version accepted")
	}
}
