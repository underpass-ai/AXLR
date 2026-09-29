package diagnostics

import (
	"path/filepath"
	"testing"

	"github.com/underpass-ai/AXLR/tui/application"
)

func TestFileLoggerReportsWriteFailureAtClose(t *testing.T) {
	logger, err := Open(filepath.Join(t.TempDir(), "trace.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if err := logger.file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := logger.Record(application.DiagnosticEvent{Stage: application.DiagnosticStartup}); err == nil {
		t.Fatal("closed file accepted a record")
	}
	if err := logger.Close(); err == nil {
		t.Fatal("close hid earlier write failure")
	}
}
