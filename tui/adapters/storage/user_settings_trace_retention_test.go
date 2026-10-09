package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUserSettingsTraceRetention(t *testing.T) {
	for _, tc := range []struct {
		json string
		want time.Duration
	}{
		{`{}`, 30 * 24 * time.Hour},
		{`{"trace_retention_days":7}`, 7 * 24 * time.Hour},
		{`{"trace_retention_days":0}`, 0},
	} {
		path := filepath.Join(t.TempDir(), "settings.json")
		if err := os.WriteFile(path, []byte(tc.json), 0600); err != nil {
			t.Fatal(err)
		}
		store, err := NewUserSettingsStore(path, DefaultUserSettings())
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		got, err := store.Load(ctx)
		if err != nil || got.TraceRetention() != tc.want || len(got.Extra) != 0 {
			t.Fatalf("%s: retention=%v extra=%v err=%v", tc.json, got.TraceRetention(), got.Extra, err)
		}
		// Saving another setting keeps an explicit value, 0 included.
		if err := store.ModelPreference().Save(ctx, "provider/picked"); err != nil {
			t.Fatal(err)
		}
		got, err = store.Load(ctx)
		if err != nil || got.TraceRetention() != tc.want {
			t.Fatalf("%s after save: retention=%v err=%v", tc.json, got.TraceRetention(), err)
		}
		data, _ := os.ReadFile(path)
		if strings.Count(string(data), "trace_retention_days") != strings.Count(tc.json, "trace_retention_days") {
			t.Fatalf("%s saved as %s", tc.json, data)
		}
	}
	for _, invalid := range []string{`{"trace_retention_days":-1}`, `{"trace_retention_days":36501}`, `{"trace_retention_days":"30"}`} {
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
