package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRepairInstallIsOffUnlessSettingsTurnItOn(t *testing.T) {
	if DefaultUserSettings().RepairConfiguration().Install {
		t.Fatal("repair.install is on by default")
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"repair":{"repository":"o/r","auto_merge":false,"install":true}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := NewUserSettingsStore(path, DefaultUserSettings())
	if err != nil {
		t.Fatal(err)
	}
	settings, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if repair := settings.RepairConfiguration(); !repair.Install || repair.Repository != "o/r" {
		t.Fatalf("repair = %+v", repair)
	}
}
