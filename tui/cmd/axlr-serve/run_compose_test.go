package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// serveConfig writes a complete service configuration whose engines are
// never reached: the manager connects on first use.
func serveConfig(t *testing.T, settings string) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name string, data []byte) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "axlr-serve-test"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	private, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certificate := write("tls.crt", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	policy, _ := json.Marshal(map[string]any{"version": 1, "entries": []any{map[string]any{"certificate_sha256": strings.Repeat("ab", 32), "principal_id": "bot", "roles": []string{"session_client"}}}})
	workspace := filepath.Join(dir, "workspace")
	if err := os.Mkdir(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	engine := func(port string) map[string]string {
		return map[string]string{"endpoint": "127.0.0.1:" + port, "server_name": "engine.test", "tls_dir": dir, "command": filepath.Join(dir, "missing-engine")}
	}
	config := map[string]any{
		"api_listen": "127.0.0.1:0", "probe_listen": "127.0.0.1:0", "workspace": workspace, "state_dir": filepath.Join(dir, "state"),
		"server_cert_file": certificate, "server_key_file": write("tls.key", pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: private})), "client_ca_file": certificate,
		"principals_file": write("principals.json", policy), "model_api_key_file": write("model-key", []byte("test-key\n")),
		"kmp": engine("1"), "made": engine("2"),
	}
	if settings != "" {
		config["settings_file"] = write("settings.json", []byte(settings))
	}
	data, _ := json.Marshal(config)
	return write("service.json", data)
}

// The service composes the console's loop and starts: with the defaults,
// and with a settings file that names a remote OpenAI-compatible model by
// its key variable, the way Grok is reached without OpenRouter.
func TestRunComposesTheLoopAndStarts(t *testing.T) {
	t.Setenv("XAI_API_KEY", "xai-never-print")
	for name, settings := range map[string]string{
		"defaults": "",
		"settings": `{"exec_sandbox":{"mode":"off"},"forged_tools":false,"max_session_usd":1,"local_models":[{"id":"xai/grok","url":"https://api.x.ai/v1","model":"grok-4","api_key_env":"XAI_API_KEY","context_tokens":131072}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := serveConfig(t, settings)
			ctx, cancel := context.WithCancel(context.Background())
			timer := time.AfterFunc(500*time.Millisecond, cancel)
			defer timer.Stop()
			var stdout, stderr bytes.Buffer
			code := run(ctx, []string{"--config", path}, &stdout, &stderr)
			output := stderr.String()
			t.Log(output)
			for _, failure := range []string{"invalid", "unavailable:", "state unavailable", "workspace unavailable", "provider unavailable", "exec sandbox:", "xai-never-print"} {
				if strings.Contains(output, failure) {
					t.Fatalf("stderr has %q:\n%s", failure, output)
				}
			}
			if code != 0 {
				t.Fatalf("code %d:\n%s", code, output)
			}
		})
	}
}

// A settings file the configuration names must exist.
func TestRunRefusesAMissingSettingsFile(t *testing.T) {
	path := serveConfig(t, "{}")
	data, _ := os.ReadFile(path)
	var config map[string]any
	_ = json.Unmarshal(data, &config)
	_ = os.Remove(config["settings_file"].(string))
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"--config", path}, &stdout, &stderr); code == 0 || !strings.Contains(stderr.String(), "settings unavailable") {
		t.Fatalf("code %d: %s", code, &stderr)
	}
}
