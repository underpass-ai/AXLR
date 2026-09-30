package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validConfig(dir string) Config {
	return Config{APIListen: "127.0.0.1:8443", ProbeListen: "127.0.0.1:8081", Workspace: dir, StateDir: filepath.Join(dir, "state"), ServerCertFile: filepath.Join(dir, "server.crt"), ServerKeyFile: filepath.Join(dir, "server.key"), ClientCAFile: filepath.Join(dir, "ca.crt"), PrincipalsFile: filepath.Join(dir, "principals.json"), ModelAPIKeyFile: filepath.Join(dir, "model-key"), KMP: EngineConfig{Endpoint: "kmp.example:443", ServerName: "kmp.example", TLSDir: filepath.Join(dir, "kmp"), Command: "/bin/kmp-mcp"}, MADE: EngineConfig{Endpoint: "made.example:443", ServerName: "made.example", TLSDir: filepath.Join(dir, "made"), Command: "/bin/made-mcp"}}
}

func TestConfigRejectsUnknownFieldsAndUnsafeListeners(t *testing.T) {
	dir := t.TempDir()
	cfg := validConfig(dir)
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "service.json")
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err != nil {
		t.Fatal(err)
	}
	bad := append(data[:len(data)-1], []byte(`,"api_key":"literal-secret"}`)...)
	if err := os.WriteFile(path, bad, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("accepted unknown secret field")
	}
	for _, listen := range []string{"0.0.0.0:8081", "localhost:8081", "invalid"} {
		cfg.ProbeListen = listen
		if err := cfg.Validate(); err == nil {
			t.Fatalf("accepted probe listener %q", listen)
		}
	}
}

func TestRemoteRegistrationHasCompleteMTLSEnvironment(t *testing.T) {
	cfg := validConfig(t.TempDir())
	registrations, profiles, err := EngineRegistrations(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(registrations) != 2 || len(profiles) != 2 || profiles[0].Purpose != "memory" || profiles[1].Purpose != "ceremony" {
		t.Fatalf("profiles: %+v", profiles)
	}
	for _, reg := range registrations {
		if !reg.Manifest.AllowAll || !strings.HasPrefix(reg.Manifest.Command, "/bin/") || len(reg.Env) != 7 {
			t.Fatalf("registration: %+v", reg)
		}
		joined := strings.Join(reg.Env, "\n")
		for _, suffix := range []string{"_TLS_MODE=mutual", "_TLS_CA_PATH=", "_TLS_CERT_PATH=", "_TLS_KEY_PATH=", "_TLS_DOMAIN_NAME=", "_ENDPOINT="} {
			if !strings.Contains(joined, suffix) {
				t.Fatalf("missing %s: %s", suffix, joined)
			}
		}
		if strings.Contains(joined, "BACKEND=embedded") {
			t.Fatal("embedded fallback")
		}
	}
}

func TestPrincipalPolicyRejectsDuplicateAndUnknownRoles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "principals.json")
	fingerprint := strings.Repeat("a", 64)
	write := func(entries []any) {
		t.Helper()
		data, _ := json.Marshal(map[string]any{"version": 1, "entries": entries})
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	entry := func(role string) any {
		return map[string]any{"certificate_sha256": fingerprint, "principal_id": "alice", "roles": []string{role}}
	}
	write([]any{entry("session_client")})
	policy, err := loadPrincipals(path)
	if err != nil || !policy[fingerprint].Can("session_client") || policy[fingerprint].Can("approver") {
		t.Fatalf("valid policy: %v %+v", err, policy)
	}
	write([]any{entry("owner")})
	if _, err := loadPrincipals(path); err == nil {
		t.Fatal("unknown role accepted")
	}
	write([]any{entry("session_client"), entry("approver")})
	if _, err := loadPrincipals(path); err == nil {
		t.Fatal("duplicate certificate accepted")
	}
}
