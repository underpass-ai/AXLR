package storage

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCredentialsRoundTripPrivately(t *testing.T) {
	store := CredentialsStore{Path: filepath.Join(t.TempDir(), "axlr", "credentials.json")}
	if credentials, err := store.Load(); err != nil || credentials.OpenRouterAPIKey != "" {
		t.Fatalf("missing file: %+v %v", credentials, err)
	}
	if err := store.SaveOpenRouterKey(context.Background(), "sk-or-v1-abc"); err != nil {
		t.Fatal(err)
	}
	credentials, err := store.Load()
	if err != nil || credentials.OpenRouterAPIKey != "sk-or-v1-abc" {
		t.Fatalf("%+v %v", credentials, err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(store.Path)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("mode %v %v", info.Mode(), err)
		}
	}
}

func TestCredentialsRefuseInvalidKeysAndFiles(t *testing.T) {
	dir := t.TempDir()
	store := CredentialsStore{Path: filepath.Join(dir, "credentials.json")}
	for _, key := range []string{"", " sk", "sk or", "sk\n"} {
		if err := store.SaveOpenRouterKey(context.Background(), key); err == nil {
			t.Fatalf("%q accepted", key)
		}
	}
	for name, body := range map[string]string{"json": "{", "unknown": `{"other":"x"}`, "spaces": `{"openrouter_api_key":"a b"}`} {
		if err := os.WriteFile(store.Path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Load(); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	if runtime.GOOS == "windows" {
		return
	}
	if err := os.WriteFile(store.Path, []byte(`{"openrouter_api_key":"sk"}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(store.Path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Load(); err == nil || !strings.Contains(err.Error(), "chmod 600") {
		t.Fatalf("readable credentials accepted: %v", err)
	}
	if _, err := (CredentialsStore{Path: "relative.json"}).Load(); err == nil {
		t.Fatal("relative path accepted")
	}
}
