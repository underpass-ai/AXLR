package ceremonyhost

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/mcpclient"
)

// TestPinnedDigestsMatchTheShippedYAML publishes each embedded definition in a
// disposable MADE store and compares MADE's digest with the pin. It needs a
// made-mcp binary; set AXLR_MADE_MCP or put made-mcp on PATH.
func TestPinnedDigestsMatchTheShippedYAML(t *testing.T) {
	binary := os.Getenv("AXLR_MADE_MCP")
	if binary == "" {
		binary, _ = exec.LookPath("made-mcp")
	}
	if binary == "" {
		t.Skip("made-mcp not available")
	}
	dir := t.TempDir()
	store := filepath.Join(dir, "pin.sqlite3")
	const host = "axlr-pin-test"
	if out, err := exec.Command(binary, "bootstrap-authorization", store, "--policy-id", host, "--trusted-host-id", host).CombinedOutput(); err != nil {
		t.Fatalf("bootstrap: %v %s", err, out)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	env := []string{"MADE_MCP_BACKEND=embedded", "MADE_MCP_STORE_PATH=" + store, "MADE_AUTH_POLICY_ID=" + host, "MADE_AUTH_TRUSTED_HOST_ID=" + host, "MADE_CEREMONY_STORE_ID=" + host, "MADE_CEREMONY_SEARCH_CURSOR_HMAC_KEY=" + hex.EncodeToString(key), "HOME=" + dir}
	server := mcpclient.Server{Name: "made", Command: binary, Env: env}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	call := func(tool string, arguments map[string]any) map[string]any {
		client := mcpclient.New()
		defer client.Close()
		if err := client.Connect(ctx, server); err != nil {
			t.Fatal(err)
		}
		name, _ := mcpclient.NewToolName(tool)
		result, err := client.Call(ctx, mcpclient.ToolRef{Server: "made", Name: name}, arguments)
		if err != nil || result.IsError {
			t.Fatalf("%s: %v %s", tool, err, result.Content)
		}
		return nil
	}
	call("made_issue_authorization_grant", map[string]any{"grant_id": "pin", "grantee_id": host, "scope": map[string]any{"kind": "global"}, "valid_from": "2000-01-01T00:00:00Z", "delegation_depth": 0, "actions": []string{"publish_ceremony_definition", "get_ceremony_definition"}})
	for _, d := range Definitions() {
		text, err := d.YAML()
		if err != nil {
			t.Fatal(err)
		}
		client := mcpclient.New()
		if err := client.Connect(ctx, server); err != nil {
			t.Fatal(err)
		}
		name, _ := mcpclient.NewToolName("made_publish_ceremony_definition")
		result, err := client.Call(ctx, mcpclient.ToolRef{Server: "made", Name: name}, map[string]any{"definition_yaml": text})
		client.Close()
		if err != nil || result.IsError {
			t.Fatalf("publish %s: %v %s", d.Name, err, result.Content)
		}
		if !containsDigest(result.StructuredContent, d.Digest) {
			t.Fatalf("%s %s: pinned digest %s no longer matches the YAML: %s", d.Name, d.Version, d.Digest, result.StructuredContent)
		}
	}
}

func containsDigest(raw []byte, digest string) bool {
	return strings.Contains(string(raw), digest)
}
