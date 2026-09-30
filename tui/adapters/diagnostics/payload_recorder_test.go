package diagnostics

import (
	"bytes"
	"os"
	"path/filepath"
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
	_ = os.Chmod(target, 0755)
	if _, err := NewPayloadRecorder(target); err == nil {
		t.Fatal("public directory accepted")
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
