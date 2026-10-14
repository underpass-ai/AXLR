package terminal

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

// On 14 October 2026, after "Editor: exit status 1", Ctrl+C cleared the
// draft and the next Ctrl+C's "press Ctrl+C again to quit" never showed:
// the error stayed in its place. A notice displaces an error an earlier
// update left, and shows beside one that must stay (an unsaved reply).
func TestAQuitHintIsNeverHiddenByAnError(t *testing.T) {
	m := sized()
	m.Status.Error = "Editor: exit status 1 · the draft did not change"
	m.Composer.Input.SetValue("draft")
	m = update(m, ctrlC)
	if m.Composer.Input.Value() != "" {
		t.Fatal("setup: the first Ctrl+C kept the draft")
	}
	m = update(m, ctrlC)
	if footer := ansi.Strip(m.footerView()); !strings.Contains(footer, "press Ctrl+C again to quit") || m.Status.Error != "" {
		t.Fatalf("stale error %q, footer %q", m.Status.Error, footer)
	}
	kept := sized()
	kept.draft = "partial reply"
	kept.Status.Error = "save failed"
	kept = update(kept, ctrlC)
	footer := ansi.Strip(kept.footerView())
	if !strings.Contains(footer, "press Ctrl+C again to quit") || !strings.Contains(footer, "save failed") {
		t.Fatalf("an unsaved reply's error hid the quit hint: %q", footer)
	}
	if status := ansi.Strip(kept.statusView()); !strings.Contains(status, "press Ctrl+C again to quit") {
		t.Fatalf("under an overlay the error hid the quit hint: %q", status)
	}
}

// After a failed restore, u in Ctrl+D asked again at the top of the panel
// while the old error stayed at the bottom.
func TestAQuestionDisplacesAStaleError(t *testing.T) {
	m, files := restoreModel(t)
	defer m.zones.Close()
	m = update(m, ControlIntent("changes"))
	files["b.go"] = "edited by hand\n"
	m = update(m, typed('u'))
	next, cmd := m.Update(typed('y'))
	m = drain(t, next.(AppModel), cmd)
	if m.Status.Error == "" {
		t.Fatal("setup: the restore did not fail")
	}
	if m = update(m, typed('u')); m.Changes.confirm == "" || m.Status.Error != "" {
		t.Fatalf("confirm %q, error %q", m.Changes.confirm, m.Status.Error)
	}
}

// A notice goes after its life, at the next tick or redraw; /undo's
// question and the budget warning stay until a key.
func TestANoticeExpires(t *testing.T) {
	m := sized()
	m.Composer.Input.SetValue("/new now")
	m = update(m, ControlIntent("send"))
	if m.Status.Notice == "" || m.noticeAt.IsZero() {
		t.Fatalf("setup: no dated notice (%q)", m.Status.Notice)
	}
	if next, cmd := m.Update(noticeTick{}); next.(AppModel).Status.Notice == "" || cmd == nil {
		t.Fatal("a fresh notice expired, or the tick was not rearmed")
	}
	m.noticeAt = time.Now().Add(-noticeLife)
	if strings.Contains(ansi.Strip(m.footerView()), "/new") {
		t.Fatal("a redraw kept an old notice")
	}
	if next, cmd := m.Update(noticeTick{}); next.(AppModel).Status.Notice != "" || cmd == nil {
		t.Fatal("the tick kept an old notice")
	}
	m.undoAsking = true
	if next, _ := m.Update(noticeTick{}); next.(AppModel).Status.Notice == "" {
		t.Fatal("the tick ended /undo's question")
	}
	m.undoAsking = false
	m.lastingNotice = m.Status.Notice
	if next, _ := m.Update(noticeTick{}); next.(AppModel).Status.Notice == "" {
		t.Fatal("the tick ended the budget warning")
	}
}
