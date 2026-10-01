package storage

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestApprovalSettingsPersistExactToolsAndAutonomy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "approvals.json")
	settings, err := NewApprovalSettings(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	read, _ := domain.NewLocalToolIdentity("read")
	write, _ := domain.NewLocalToolIdentity("write")
	if settings.AutoApproves(write) || settings.Autonomous() {
		t.Fatal("new policy must require write approval")
	}
	if err := settings.Allow(context.Background(), read); err != nil {
		t.Fatal(err)
	}
	if !settings.AutoApproves(read) || settings.AutoApproves(write) {
		t.Fatal("allow must match exact identity")
	}
	if err := settings.SetAutonomous(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewApprovalSettings(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.Autonomous() || !reloaded.AutoApproves(write) || len(reloaded.Allowed()) != 1 {
		t.Fatal("saved policy did not survive restart")
	}
	if err := reloaded.SetAutonomous(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if reloaded.AutoApproves(write) || !reloaded.AutoApproves(read) {
		t.Fatal("turning autonomy off lost exact approval or kept broad approval")
	}
}

func TestApprovalSettingsReadAndWriteUserSettingsJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	read, _ := domain.NewLocalToolIdentity("read")
	write, _ := domain.NewLocalToolIdentity("write")
	initial := DefaultUserSettings()
	initial.Approvals = &ApprovalPreferences{Autonomous: true, Allowed: []domain.ToolIdentity{read}, Extra: map[string]json.RawMessage{"future_policy": json.RawMessage(`{"mode":"next"}`)}}
	data, err := json.Marshal(initial)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	store, _ := NewUserSettingsStore(path, DefaultUserSettings())
	settings, err := NewApprovalSettingsInUserSettings(store, filepath.Join(dir, "approvals.json"), nil)
	if err != nil || !settings.Autonomous() || !settings.AutoApproves(write) || len(settings.Allowed()) != 1 {
		t.Fatalf("editable approvals not loaded: %+v, %v", settings, err)
	}
	if err := settings.SetAutonomous(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if err := settings.Allow(context.Background(), write); err != nil {
		t.Fatal(err)
	}
	user, err := store.Load(context.Background())
	if err != nil || user.Approvals == nil || user.Approvals.Autonomous || len(user.Approvals.Allowed) != 2 {
		t.Fatalf("approvals not saved in settings.json: %+v, %v", user.Approvals, err)
	}
	var future struct {
		Mode string `json:"mode"`
	}
	if err := json.Unmarshal(user.Approvals.Extra["future_policy"], &future); err != nil || future.Mode != "next" {
		t.Fatalf("future approval setting lost: %+v, %v", future, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "approvals.json")); !os.IsNotExist(err) {
		t.Fatalf("old approval file was written: %v", err)
	}
}

func TestApprovalSettingsMigrateLegacyFileOnNextChange(t *testing.T) {
	dir := t.TempDir()
	legacyPath := filepath.Join(dir, "approvals.json")
	legacy, err := NewApprovalSettings(legacyPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	read, _ := domain.NewLocalToolIdentity("read")
	if err := legacy.Allow(context.Background(), read); err != nil {
		t.Fatal(err)
	}
	if err := legacy.SetAutonomous(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	store, _ := NewUserSettingsStore(filepath.Join(dir, "settings.json"), DefaultUserSettings())
	settings, err := NewApprovalSettingsInUserSettings(store, legacyPath, nil)
	if err != nil || !settings.Autonomous() || !settings.AutoApproves(read) {
		t.Fatalf("legacy approvals not loaded: %+v, %v", settings, err)
	}
	if err := settings.SetAutonomous(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	user, err := store.Load(context.Background())
	if err != nil || user.Approvals == nil || user.Approvals.Autonomous || len(user.Approvals.Allowed) != 1 || user.Approvals.Allowed[0] != read {
		t.Fatalf("legacy approvals not migrated: %+v, %v", user.Approvals, err)
	}
}

func TestApprovalSettingsRejectCancelledMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "approvals.json")
	settings, err := NewApprovalSettings(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	write, _ := domain.NewLocalToolIdentity("write")
	if settings.Allow(ctx, write) == nil || settings.AutoApproves(write) {
		t.Fatal("cancelled approval took effect")
	}
	if settings.SetAutonomous(ctx, true) == nil || settings.Autonomous() {
		t.Fatal("cancelled autonomous change took effect")
	}
}
