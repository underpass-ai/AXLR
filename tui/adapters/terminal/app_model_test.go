package terminal

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func update(m AppModel, msg tea.Msg) AppModel { n, _ := m.Update(msg); return n.(AppModel) }
func sized() AppModel {
	session, err := domain.NewSession("0123456789abcdef0123456789abcdef", "/tmp", "model")
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
func TestAppModelMultilineAndSend(t *testing.T) {
	m := sized()
	m.Composer.Input.SetValue("first")
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	m = update(m, tea.KeyPressMsg{Code: 'x', Text: "x"})
	if m.Composer.Input.Value() != "first\nx" {
		t.Fatal(m.Composer.Input.Value())
	}
	m = update(m, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if !m.Busy || m.Composer.Input.Value() != "" {
		t.Fatal("send did not start operation")
	}
	m.Composer.Input.SetValue("second")
	m = update(m, tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl})
	if m.Composer.Input.Value() != "second" {
		t.Fatal("busy operation accepted another turn")
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
	m = update(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if !m.ActivityTab {
		t.Fatal("tab did not select activity")
	}
	m = update(m, ControlIntent("transcript"))
	if m.ActivityTab {
		t.Fatal("control did not select transcript")
	}
	m.View()
	z := m.zones.Get(m.prefix + "activity")
	for deadline := time.Now().Add(time.Second); z == nil && time.Now().Before(deadline); {
		time.Sleep(time.Millisecond)
		z = m.zones.Get(m.prefix + "activity")
	}
	if z == nil {
		t.Fatal("activity has no clickable zone")
	}
	m = update(m, tea.MouseClickMsg{X: z.StartX, Y: z.StartY, Button: tea.MouseLeft})
	if !m.ActivityTab {
		t.Fatal("mouse did not select activity")
	}
}

func TestAppModelOperationPublishesOnlyOnCompletion(t *testing.T) {
	session, err := domain.NewSession("0123456789abcdef0123456789abcdef", "/tmp", "model")
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

func TestAppModelNoColorEnvironment(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	m := update(New(Dependencies{}), tea.WindowSizeMsg{Width: 100, Height: 30})
	if strings.Contains(m.View().Content, "\x1b") {
		t.Fatal("NO_COLOR ignored")
	}
}
func TestAppModelChildSendIntent(t *testing.T) {
	c := NewComposer(true)
	if c.Intent(tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}) != ControlIntent("send") {
		t.Fatal("composer did not emit send intent")
	}
	if c.Intent(tea.KeyPressMsg{Code: tea.KeyEnter}) != "" {
		t.Fatal("newline emits action")
	}
}

func TestAppModelMonochromeHasVisibleEditorCursor(t *testing.T) {
	m := sized()
	v := m.View()
	if v.Cursor == nil || v.Cursor.Y < 24 || v.Cursor.Y > 26 {
		t.Fatal("editor cursor missing or misplaced")
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
