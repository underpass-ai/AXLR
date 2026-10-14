package terminal

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// On 14 October 2026 a session imported into another workspace listed the
// origin's file changes in Ctrl+D as its own and offered to restore them.
func TestChangesOfAnImportedSessionAreMarkedAndNotRestored(t *testing.T) {
	m, files := restoreModel(t)
	defer m.zones.Close()
	labels, err := storage.NewSessionLabelStore(filepath.Join(t.TempDir(), "session-labels.json"))
	if err != nil {
		t.Fatal(err)
	}
	origin := domain.Workspace(filepath.Join(t.TempDir(), "ws"))
	if _, err := labels.Initialize(context.Background(), m.Header.State.ID, domain.SessionLabel{ImportedFrom: origin, ImportedAfter: len(m.Header.State.Messages)}); err != nil {
		t.Fatal(err)
	}
	m.deps.SessionLabels = labels
	m.Changes.importFor = ""
	m.refreshTranscript()
	m = update(m, ControlIntent("changes"))
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "other workspace") || !strings.Contains(view, "Made in "+shortenHome(string(origin))[:20]) {
		t.Fatalf("the changes are not marked as made elsewhere:\n%s", view)
	}
	m = update(m, typed('u'))
	if m.Changes.confirm != "" || !strings.Contains(m.Changes.notice, "before this session was imported here") {
		t.Fatalf("u offered to restore another workspace's change: confirm=%q notice=%q", m.Changes.confirm, m.Changes.notice)
	}
	m = update(m, ControlIntent("close"))
	m.overlay = ""
	m.Composer.Input.SetValue("/undo")
	m = update(m, ControlIntent("send"))
	if m.undoAsking || !strings.Contains(m.Status.Notice, "restore it from a console opened there") {
		t.Fatalf("/undo offered to restore another workspace's turn: %q", m.Status.Notice)
	}
	if files["a.go"] != "three\n" || files["b.go"] != "y\n" {
		t.Fatal("a file was written")
	}
	// A session that was not imported is not marked.
	plain, _ := restoreModel(t)
	defer plain.zones.Close()
	plain = update(plain, ControlIntent("changes"))
	if strings.Contains(ansi.Strip(plain.View().Content), "other workspace") {
		t.Fatal("a local session's changes are marked as made elsewhere")
	}
}
