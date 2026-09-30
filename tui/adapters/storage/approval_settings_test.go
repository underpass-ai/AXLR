package storage

import (
	"context"
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
