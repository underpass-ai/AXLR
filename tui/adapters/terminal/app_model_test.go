package terminal

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func testWorkspace() domain.Workspace {
	return domain.Workspace(filepath.VolumeName(os.TempDir()) + string(filepath.Separator) + "axlr-test")
}

func update(m AppModel, msg tea.Msg) AppModel { n, _ := m.Update(msg); return n.(AppModel) }
func sized() AppModel {
	session, err := domain.NewSession("0123456789abcdef0123456789abcdef", testWorkspace(), "model")
	if err != nil {
		panic(err)
	}
	return update(New(Dependencies{Session: &session, Monochrome: true}), tea.WindowSizeMsg{Width: 100, Height: 30})
}
func TestAppModelStreamRendering(t *testing.T) {
	m := sized()
	m = update(m, application.Event{Kind: application.EventTextDelta, Text: "hello 界"})
	if !strings.Contains(m.View().Content, "hello 界") {
		t.Fatal(m.View().Content)
	}
	if !m.View().AltScreen || m.View().MouseMode != tea.MouseModeCellMotion {
		t.Fatal("terminal modes absent")
	}
}
func TestAppModelRequestsTerminalBackground(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	if cmd := New(Dependencies{}).Init(); cmd == nil {
		t.Fatal("terminal background is never queried")
	}
}

func TestAppModelKeepsUnpersistedDraftOnFailureAndClearsOldError(t *testing.T) {
	m := sized()
	m.Busy = true
	m.operationID = 1
	m.draft = "partial answer"
	m.draftOperationID = 1
	m.Status.Error = "previous error"
	m = update(m, operationComplete{ID: 1, Session: *m.deps.Session, Err: errors.New("save failed")})
	if m.draft != "partial answer" || !strings.Contains(m.View().Content, "partial answer") {
		t.Fatal("failed save lost visible draft")
	}
	m.Busy = true
	m.operationID = 2
	m = update(m, operationComplete{ID: 2, Session: *m.deps.Session})
	if m.Status.Error != "save failed" {
		t.Fatal("unrelated successful operation hid unsaved draft error")
	}
	if m.draft != "partial answer" {
		t.Fatal("unrelated successful operation lost an unpersisted draft")
	}
}

func TestAppModelSuccessfulOperationClearsOldErrorWithoutUnsavedDraft(t *testing.T) {
	m := sized()
	m.Busy = true
	m.operationID = 1
	m.Status.Error = "old error"
	m = update(m, operationComplete{ID: 1, Session: *m.deps.Session})
	if m.Status.Error != "" {
		t.Fatal("successful operation retained an unrelated old error")
	}
}

func TestAppModelDoesNotMatchDraftAgainstOldAnswerAndClearsOnSessionSwitch(t *testing.T) {
	m := sized()
	m.Header.State.Messages = append(m.Header.State.Messages, root.Message{Role: root.RoleAssistant, Content: "same answer"})
	m.Busy = true
	m.operationID = 1
	m.draft = "same answer"
	m.draftOperationID = 1
	m.operationMessages = 1
	m = update(m, operationComplete{ID: 1, Session: *m.deps.Session, Err: errors.New("save failed")})
	if m.draft != "same answer" {
		t.Fatal("old assistant answer matched current unpersisted draft")
	}
	other, err := domain.NewSession("fedcba9876543210fedcba9876543210", testWorkspace(), "model")
	if err != nil {
		t.Fatal(err)
	}
	m.Busy = true
	m.operationID = 2
	m = update(m, operationComplete{ID: 2, Session: other})
	if m.draft != "" || strings.Contains(m.View().Content, "same answer") {
		t.Fatal("unpersisted draft leaked to another session")
	}
}

func TestAppModelRetryStartsFreshDraftWithoutDroppingPreviousBeforeDelta(t *testing.T) {
	m := sized()
	m.draft = "old partial"
	m.draftOperationID = 1
	m.operationID = 1
	m.Header.State.Status = domain.StatusStreaming
	m = update(m, ControlIntent("continue"))
	if m.draft != "old partial" || !m.Busy {
		t.Fatal("retry discarded old output before replacement arrived")
	}
	m = update(m, application.Event{Kind: application.EventTextDelta, Text: "new answer"})
	if m.draft != "new answer" || strings.Contains(m.View().Content, "old partial") {
		t.Fatal("retry concatenated old and new output")
	}
	m.cancel()
}

func TestAppModelRejectsNewPromptWhileSessionIsStreaming(t *testing.T) {
	m := sized()
	m.Header.State.Status = domain.StatusStreaming
	m.draft = "unpersisted answer"
	m.Composer.Input.SetValue("another prompt")
	m = update(m, ControlIntent("send"))
	if m.Busy || m.draft != "unpersisted answer" || m.Composer.Input.Value() != "another prompt" {
		t.Fatal("invalid new prompt cleared the unpersisted output")
	}
}

func TestAppModelKeepsSecondStreamDraftWhenItsSaveFails(t *testing.T) {
	m := sized()
	m.Busy = true
	m.operationID = 1
	m.operationMessages = 1
	m = update(m, application.Event{Kind: application.EventStreamStart, MessageCount: 1})
	m = update(m, application.Event{Kind: application.EventTextDelta, Text: "first saved answer"})
	m = update(m, application.Event{Kind: application.EventStreamStart, MessageCount: 3})
	m = update(m, application.Event{Kind: application.EventTextDelta, Text: "second unsaved answer"})
	if m.draft != "second unsaved answer" {
		t.Fatalf("stream boundary did not reset draft: %q", m.draft)
	}
	private := *m.deps.Session
	if err := private.BeginTurn("question", nil); err != nil {
		t.Fatal(err)
	}
	if err := private.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "first saved answer"}}); err != nil {
		t.Fatal(err)
	}
	m = update(m, operationComplete{ID: 1, Session: private, Err: errors.New("second save failed")})
	if m.draft != "second unsaved answer" || !strings.Contains(m.View().Content, "second unsaved answer") {
		t.Fatal("first saved answer hid the second unsaved draft")
	}
}
func TestAppModelMultilineAndSend(t *testing.T) {
	m := sized()
	m.Composer.Input.SetValue("first")
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	m = update(m, tea.KeyPressMsg{Code: 'x', Text: "x"})
	if m.Composer.Input.Value() != "first\nx" {
		t.Fatal(m.Composer.Input.Value())
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !m.Busy || m.Composer.Input.Value() != "" {
		t.Fatal("send did not start operation")
	}
	m.Composer.Input.SetValue("second")
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.Composer.Input.Value() != "" || m.steer.Peek() != "second" {
		t.Fatal("busy operation did not queue steering message")
	}
	m.cancel()
}
func TestAppModelResizePreservesEditorAndScroll(t *testing.T) {
	m := sized()
	m.Composer.Input.SetValue("draft\nsecond")
	m.Transcript.SetContent(strings.Repeat("line\n", 80))
	m.Transcript.Viewport.SetYOffset(7)
	m = update(m, tea.WindowSizeMsg{Width: 70, Height: 20})
	if m.Composer.Input.Value() != "draft\nsecond" || !m.Composer.Input.Focused() || m.Transcript.Viewport.YOffset() != 7 {
		t.Fatal("resize lost state")
	}
}
func TestAppModelResizeKeepsTheLatestTurnInView(t *testing.T) {
	m := sized()
	m.Transcript.SetContent(strings.Repeat("palabra ", 2000))
	m.Transcript.Viewport.GotoBottom()
	m = update(m, tea.WindowSizeMsg{Width: 50, Height: 20})
	if !m.Transcript.Viewport.AtBottom() {
		t.Fatalf("narrower rewrap left the view at offset %d of %d lines", m.Transcript.Viewport.YOffset(), m.Transcript.VisualLineCount())
	}
}
func TestAppModelSanitizationAndMonochrome(t *testing.T) {
	m := sized()
	m = update(m, application.Event{Kind: application.EventTextDelta, Text: "safe\x1b[2J\x1b]52;c;secret\a\x00\x08text"})
	got := m.View().Content
	if strings.ContainsAny(got, "\x1b\x00\x08\a") || strings.Contains(got, "secret") || !strings.Contains(got, "safetext") {
		t.Fatalf("unsafe: %q", got)
	}
}
func TestAppModelVisibleControlsKeyboardMouseParity(t *testing.T) {
	m := update(sized(), tea.WindowSizeMsg{Width: 70, Height: 20})
	m = update(m, tea.KeyPressMsg{Code: tea.KeyF1})
	if m.overlay != "help" {
		t.Fatal("F1 did not open help")
	}
	m = update(m, ControlIntent("close"))
	m.View()
	z := m.zones.Get(m.prefix + "help")
	for deadline := time.Now().Add(time.Second); z == nil && time.Now().Before(deadline); {
		time.Sleep(time.Millisecond)
		z = m.zones.Get(m.prefix + "help")
	}
	if z == nil {
		t.Fatal("the help hint has no clickable zone")
	}
	m = update(m, tea.MouseClickMsg{X: z.StartX, Y: z.StartY, Button: tea.MouseLeft})
	if m.overlay != "help" {
		t.Fatal("clicking the help hint did not open help")
	}
}

func TestAppModelOperationPublishesOnlyOnCompletion(t *testing.T) {
	session, err := domain.NewSession("0123456789abcdef0123456789abcdef", testWorkspace(), "model")
	if err != nil {
		t.Fatal(err)
	}
	m := update(New(Dependencies{Session: &session}), tea.WindowSizeMsg{Width: 100, Height: 30})
	cmd := m.BeginOperation(func(ctx context.Context, s *domain.Session, emit func(application.Event) error) error {
		if err := s.BeginTurn("question", nil); err != nil {
			return err
		}
		return emit(application.Event{Kind: application.EventTextDelta, Text: "answer"})
	})
	if m.BeginOperation(func(context.Context, *domain.Session, func(application.Event) error) error {
		t.Error("concurrent operation")
		return nil
	}) != nil {
		t.Fatal("accepted concurrent operation")
	}
	first := cmd()
	if session.Status() != domain.StatusIdle {
		t.Fatal("worker mutated shared session")
	}
	n, next := m.Update(first)
	m = n.(AppModel)
	if !strings.Contains(m.View().Content, "answer") || next == nil {
		t.Fatal("event missing or channel not rearmed")
	}
	m = update(m, next())
	if m.Busy || session.Status() != domain.StatusStreaming || len(session.Messages()) != 1 {
		t.Fatal("completion not published")
	}
}
func TestAppModelOperationCancelAndError(t *testing.T) {
	m := sized()
	cmd := m.BeginOperation(func(ctx context.Context, _ *domain.Session, _ func(application.Event) error) error {
		<-ctx.Done()
		return ctx.Err()
	})
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	m = update(m, cmd())
	if m.Busy || !strings.Contains(m.View().Content, "context canceled") {
		t.Fatal("cancellation not visible")
	}
}
func TestAppModelSendShowsPromptWhileStreaming(t *testing.T) {
	m := sized()
	m.Composer.Input.SetValue("my prompt")
	m = update(m, ControlIntent("send"))
	defer m.cancel()
	if !strings.Contains(m.View().Content, "my prompt") {
		t.Fatal("submitted prompt disappeared")
	}
}

func TestAppModelOptimisticPromptKeepsPreviousInterruptedRowInOrder(t *testing.T) {
	s, err := domain.NewSession("0123456789abcdef0123456789abcdef", testWorkspace(), "model")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn("first prompt", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.InterruptDraft("first partial answer"); err != nil {
		t.Fatal(err)
	}
	m := update(New(Dependencies{Session: &s, Monochrome: true}), tea.WindowSizeMsg{Width: 80, Height: 24})
	m.Composer.Input.SetValue("second prompt")
	m = update(m, ControlIntent("send"))
	defer m.cancel()
	content := m.Transcript.Viewport.GetContent()
	first := strings.Index(content, "> first prompt")
	partial := strings.Index(content, "interrupted draft: first partial answer")
	second := strings.Index(content, "> second prompt")
	if first < 0 || partial <= first || second <= partial {
		t.Fatalf("optimistic rows are out of order: %q", content)
	}
}

func TestAppModelNoColorEnvironment(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := update(New(Dependencies{}), tea.WindowSizeMsg{Width: 100, Height: 30})
	if strings.Contains(m.View().Content, "\x1b") {
		t.Fatal("NO_COLOR ignored")
	}
}
func TestAppModelChildSendIntent(t *testing.T) {
	c := NewComposer(true)
	if c.Intent(tea.KeyPressMsg{Code: tea.KeyEnter}) != ControlIntent("send") {
		t.Fatal("composer did not emit send intent")
	}
	if c.Intent(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift}) != "" {
		t.Fatal("newline emits action")
	}
	if c.Intent(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}) != "" {
		t.Fatal("Ctrl+S still sends")
	}
}

func TestAppModelMonochromeHasVisibleEditorCursor(t *testing.T) {
	m := sized()
	v := m.View()
	if v.Cursor == nil {
		t.Fatal("editor cursor missing")
	}
	if lines := strings.Split(v.Content, "\n"); v.Cursor.Y >= len(lines) || !strings.HasPrefix(lines[v.Cursor.Y], "> ") {
		t.Fatalf("editor cursor misplaced at row %d", v.Cursor.Y)
	}
	m = update(m, tea.WindowSizeMsg{Width: 40, Height: 10})
	if m.View().Cursor != nil {
		t.Fatal("tiny view has offscreen cursor")
	}
}

func TestAppModelMouseSendAndCancel(t *testing.T) {
	m := sized()
	m.Composer.Input.SetValue("mouse prompt")
	m.View()
	for _, id := range []string{"send"} {
		z := m.zones.Get(m.prefix + id)
		for deadline := time.Now().Add(time.Second); z == nil && time.Now().Before(deadline); {
			time.Sleep(time.Millisecond)
			z = m.zones.Get(m.prefix + id)
		}
		if z == nil {
			t.Fatalf("no %s control", id)
		}
		n, cmd := m.Update(tea.MouseClickMsg{X: z.StartX, Y: z.StartY, Button: tea.MouseLeft})
		m = n.(AppModel)
		if id == "send" && (!m.Busy || cmd == nil || m.Composer.Input.Value() != "") {
			t.Fatal("mouse send differs from keyboard send")
		}
	}
	m.cancel()
	m = sized()
	cmd := m.BeginOperation(func(ctx context.Context, _ *domain.Session, _ func(application.Event) error) error {
		<-ctx.Done()
		return ctx.Err()
	})
	m.View()
	z := m.zones.Get(m.prefix + "cancel")
	for deadline := time.Now().Add(time.Second); z == nil && time.Now().Before(deadline); {
		time.Sleep(time.Millisecond)
		z = m.zones.Get(m.prefix + "cancel")
	}
	if z == nil {
		t.Fatal("no cancel control")
	}
	m = update(m, tea.MouseClickMsg{X: z.StartX, Y: z.StartY, Button: tea.MouseLeft})
	m = update(m, cmd())
	if m.Busy || !strings.Contains(m.View().Content, "context canceled") {
		t.Fatal("mouse cancel differs from keyboard cancel")
	}

}
