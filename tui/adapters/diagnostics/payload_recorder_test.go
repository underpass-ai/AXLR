package diagnostics

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPayloadRecorderPrivateRedactedAndBounded(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "payloads")
	recorder, err := NewPayloadRecorder(dir, "test-secret")
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"content":"test-secret Bearer oauth-sensitive sk-longlonglonglongkeyvalue","tools":[]}`)
	if err := recorder.Save(1, "request", raw); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "000001-request.json")
	stored, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored, []byte("test-secret")) || bytes.Contains(stored, []byte("oauth-sensitive")) || bytes.Contains(stored, []byte("sk-longlong")) {
		t.Fatal("secret leaked")
	}
	info, _ := os.Stat(p)
	if !testMode(info, 0600) {
		t.Fatal("payload permissions")
	}
	info, _ = os.Stat(dir)
	if !testMode(info, 0700) {
		t.Fatal("directory permissions")
	}
	if err := recorder.Save(1, "request", raw); err == nil {
		t.Fatal("existing capture overwritten")
	}
	if err := recorder.Save(2, "../../elsewhere", raw); err == nil {
		t.Fatal("untrusted kind")
	}
	if err := recorder.Save(2, "response", make([]byte, payloadLimit+1)); err == nil {
		t.Fatal("oversize accepted")
	}
	if err := recorder.SaveResponse(2, raw, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "000002-response.json")); err != nil {
		t.Fatal(err)
	}
}

func TestPayloadRecorderRetainsLongSessionBeyondFormerBudget(t *testing.T) {
	recorder, err := NewPayloadRecorder(filepath.Join(t.TempDir(), "payloads"))
	if err != nil {
		t.Fatal(err)
	}
	// Keep each capture small while checking the absence of cumulative cutoff:
	// 70 one-MiB captures exceeded the former64MiB per-run budget.
	body := bytes.Repeat([]byte("x"), 1024*1024)
	for id := uint64(1); id <= 70; id++ {
		if err := recorder.SaveResponse(id, body, false); err != nil {
			t.Fatalf("capture%d: %v", id, err)
		}
	}
}
func TestPayloadRecorderRejectsUnsafeDirectory(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{link, "relative"} {
		if _, err := NewPayloadRecorder(dir); err == nil {
			t.Fatal("unsafe directory accepted")
		}
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(target, 0755)
		if _, err := NewPayloadRecorder(target); err == nil {
			t.Fatal("public directory accepted")
		}
	}
	file := filepath.Join(base, "file")
	_ = os.WriteFile(file, nil, 0600)
	if _, err := NewPayloadRecorder(file); err == nil {
		t.Fatal("regular file accepted")
	}
}

func TestPayloadRecorderLimitsRedactedFileSize(t *testing.T) {
	recorder, err := NewPayloadRecorder(filepath.Join(t.TempDir(), "payloads"), "a")
	if err != nil {
		t.Fatal(err)
	}
	body := bytes.Repeat([]byte("a"), payloadLimit/len("[redacted]")+1)
	if err := recorder.Save(1, "request", body); err == nil {
		t.Fatal("redaction expanded past capture limit")
	}
}

// Tool results carry credentials the model read: GitHub, AWS and Slack
// tokens and private keys must not reach a capture verbatim. The samples are
// fake and built from pieces so secret scanners do not flag the test source.
func TestPayloadRecorderRedactsCommonCredentialFormats(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "payloads")
	recorder, err := NewPayloadRecorder(dir)
	if err != nil {
		t.Fatal(err)
	}
	secrets := []string{
		"ghp_" + strings.Repeat("A1b2", 9),
		"gho_" + strings.Repeat("Z9y8", 9),
		"ghs_" + strings.Repeat("Q7w6", 9),
		"ghu_" + strings.Repeat("E5r4", 9),
		"ghr_" + strings.Repeat("T3y2", 9),
		"github_pat_11ABCDEFG0123456789_" + strings.Repeat("aB3", 20),
		"AKIA" + "IOSFODNN7EXAMPLE",
		"ASIA" + "Y34FZKBOKMUTVV7A",
		"xox" + "b-1234567890-0987654321-" + "AbCdEfGhIjKlMnOpQrStUvWx",
		"wJalrXUtnFEMI/" + "K7MDENG/bPxRfiCYEXAMPLEKEY",
		"je7MtGbClwBF/" + "2Zp9Utk/h3yCo8nvbEXAMPLEKEY",
		"b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQ",
		"AAAAAAAAAAEAAAAzAAAAC3NzaC1lZDI1NTE5AAAAIB",
		"MIIEowIBAAKCAQEAtruncatedbody",
	}
	result := "keep-this-text ghost_writer\n" + strings.Join(secrets[:9], "\n") +
		"\nAWS_SECRET_ACCESS_KEY=" + secrets[9] +
		"\n{\"SecretAccessKey\": \"" + secrets[10] + "\"}\n" +
		"-----BEGIN OPENSSH PRIVATE KEY-----\n" + secrets[11] + "\n" + secrets[12] + "\n-----END OPENSSH PRIVATE KEY-----\nafter the key"
	truncated := "-----BEGIN RSA PRIVATE KEY-----\n" + secrets[13]
	request, err := json.Marshal(map[string]any{"messages": []map[string]string{
		{"role": "tool", "tool_call_id": "a", "content": result},
		{"role": "tool", "tool_call_id": "b", "content": truncated},
		{"role": "user", "content": "still here"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := recorder.Save(1, "request", request); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(filepath.Join(dir, "000001-request.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range secrets {
		if bytes.Contains(saved, []byte(secret)) {
			t.Errorf("kept verbatim: %s", secret)
		}
	}
	for _, kept := range []string{"keep-this-text ghost_writer", "AWS_SECRET_ACCESS_KEY=", "SecretAccessKey", "after the key", "still here"} {
		if !bytes.Contains(saved, []byte(kept)) {
			t.Errorf("over-redacted %q: %s", kept, saved)
		}
	}
	if !json.Valid(saved) {
		t.Fatalf("redaction broke the JSON: %s", saved)
	}
}

// A model that echoes the configured key, or writes a token into a tool
// call, streams it in pieces: "sk-or" in one delta, the rest in the next.
// No single chunk holds the secret, so the capture is redacted over each
// field's text joined across chunks.
func TestPayloadRecorderRedactsSecretsSplitAcrossStreamDeltas(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "payloads")
	key := "sk-or-v1-" + "0123456789abcdef0123456789abcdef"
	recorder, err := NewPayloadRecorder(dir, key)
	if err != nil {
		t.Fatal(err)
	}
	frames := []string{
		`{"id":"gen-1","choices":[{"index":0,"delta":{"role":"assistant","content":"Your key is sk-or"}}]}`,
		`{"id":"gen-1","choices":[{"index":0,"delta":{"content":"-v1-0123456789abcdef"}}]}`,
		`{"id":"gen-1","choices":[{"index":0,"delta":{"content":"0123456789abcdef, keep it safe."}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"write","arguments":"{\"token\":\"ghp_A1b2A1b2A1b2"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"A1b2A1b2A1b2A1b2A1b2A1b2\"}"}}]}}]}`,
		`{"choices":[{"index":0,"delta":{"reasoning":"-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBg"}}]}`,
		`{"choices":[{"index":0,"delta":{"reasoning":"kqhkiG9w0BAQEFAASC\n-----END PRIVATE KEY-----"}}]}`,
		`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`,
		`[DONE]`,
	}
	wire := "data: " + strings.Join(frames, "\n\ndata: ") + "\n\n"
	if err := recorder.SaveResponse(1, []byte(wire), true); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(filepath.Join(dir, "000001-response.sse"))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"0123456789abcdef", "A1b2", "MIIEvQIBADANBg", "kqhkiG9w0BAQEFAASC"} {
		if bytes.Contains(saved, []byte(secret)) {
			t.Errorf("split secret survives: %s", secret)
		}
	}
	for _, kept := range []string{"Your key is [redacted]", ", keep it safe.", `{\"token\":\"[redacted]`, `"finish_reason":"stop"`, `"total_tokens":7`, "data: [DONE]"} {
		if !bytes.Contains(saved, []byte(kept)) {
			t.Errorf("missing %q in %s", kept, saved)
		}
	}
	for _, line := range strings.Split(string(saved), "\n") {
		if payload, ok := strings.CutPrefix(line, "data: "); ok && payload != "[DONE]" && !json.Valid([]byte(payload)) {
			t.Errorf("frame is no longer JSON: %s", line)
		}
	}
}
