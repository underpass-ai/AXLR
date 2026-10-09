package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestUserSettingsSessionBudget(t *testing.T) {
	for _, tc := range []struct {
		json string
		want float64
	}{
		{`{}`, 0},
		{`{"max_session_usd":0}`, 0},
		{`{"max_session_usd":2.5}`, 2.5},
		{`{"max_session_usd":10000}`, 10000},
	} {
		path := filepath.Join(t.TempDir(), "settings.json")
		if err := os.WriteFile(path, []byte(tc.json), 0600); err != nil {
			t.Fatal(err)
		}
		store, err := NewUserSettingsStore(path, DefaultUserSettings())
		if err != nil {
			t.Fatal(err)
		}
		got, err := store.Load(context.Background())
		if err != nil || got.MaxSessionUSD != tc.want || len(got.Extra) != 0 {
			t.Fatalf("%s: max_session_usd=%v extra=%v err=%v", tc.json, got.MaxSessionUSD, got.Extra, err)
		}
		// Saving another setting keeps the limit.
		if err := store.ModelPreference().Save(context.Background(), "provider/picked"); err != nil {
			t.Fatal(err)
		}
		if got, err := store.Load(context.Background()); err != nil || got.MaxSessionUSD != tc.want {
			t.Fatalf("%s after save: %v %v", tc.json, got.MaxSessionUSD, err)
		}
	}
	for _, invalid := range []string{`{"max_session_usd":-1}`, `{"max_session_usd":10000.01}`, `{"max_session_usd":"5"}`} {
		path := filepath.Join(t.TempDir(), "settings.json")
		if err := os.WriteFile(path, []byte(invalid), 0600); err != nil {
			t.Fatal(err)
		}
		store, err := NewUserSettingsStore(path, DefaultUserSettings())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.Load(context.Background()); err == nil {
			t.Fatalf("%s accepted", invalid)
		}
	}
}
