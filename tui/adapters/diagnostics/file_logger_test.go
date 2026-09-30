package diagnostics_test

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/tui/adapters/diagnostics"
	"github.com/underpass-ai/AXLR/tui/application"
)

func TestFileLoggerWritesPrivateJSONLWithOnlyTypedMetadata(t *testing.T) {
	path := filepath.Join(t.TempDir(), "axlr.jsonl")
	logger, err := diagnostics.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.Record(application.DiagnosticEvent{
		Stage: application.DiagnosticProviderProgress, OperationID: 7,
		Chunks: 3, Bytes: 12, ElapsedMilliseconds: 42,
		Width: 80, Height: 24, ErrorClass: application.DiagnosticErrorNone,
	}); err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %o; want 600", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	if record["stage"] != "provider_progress" || record["operation_id"] != float64(7) || record["bytes"] != float64(12) {
		t.Fatalf("unexpected record: %s", data)
	}
	if _, ok := record["timestamp"]; !ok {
		t.Fatalf("timestamp missing: %s", data)
	}
	for _, forbidden := range []string{"text", "prompt", "token", "key", "model", "url"} {
		if _, ok := record[forbidden]; ok {
			t.Fatalf("private field %q present: %s", forbidden, data)
		}
	}
}

func TestFileLoggerRejectsFIFOWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trace.fifo")
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		logger, err := diagnostics.Open(path)
		if logger != nil {
			logger.Close()
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("FIFO accepted")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("opening a FIFO blocked")
	}
}

func TestFileLoggerRejectsUnrecognizedStringFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "axlr.jsonl")
	logger, err := diagnostics.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer logger.Close()
	if err := logger.Record(application.DiagnosticEvent{Stage: application.DiagnosticStage("user prompt")}); err == nil {
		t.Fatal("accepted arbitrary stage string")
	}
	if err := logger.Record(application.DiagnosticEvent{Stage: application.DiagnosticOperationFailed, ErrorClass: application.DiagnosticErrorClass("secret error message")}); err == nil {
		t.Fatal("accepted arbitrary error string")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		t.Fatalf("rejected values written: %q", data)
	}
}

func TestFileLoggerSerializesConcurrentRecordsAndCloses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "axlr.jsonl")
	logger, err := diagnostics.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := logger.Record(application.DiagnosticEvent{Stage: application.DiagnosticOperationStarted, OperationID: uint64(i)}); err != nil {
				t.Errorf("record %d: %v", i, err)
			}
		}()
	}
	wg.Wait()
	if err := logger.Close(); err != nil {
		t.Fatal(err)
	}
	if err := logger.Record(application.DiagnosticEvent{Stage: application.DiagnosticOperationStarted}); err == nil {
		t.Fatal("record after close succeeded")
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		line := scanner.Bytes()
		if !json.Valid(line) || !strings.Contains(string(line), `"stage":"operation_started"`) {
			t.Fatalf("invalid JSONL line: %s", line)
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if count != 100 {
		t.Fatalf("records = %d; want 100", count)
	}
}
