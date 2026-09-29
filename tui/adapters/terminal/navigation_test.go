package terminal

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"context"
	"errors"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
	"strings"
	"testing"
	"time"
)

func navSession(t *testing.T) domain.Session {
	t.Helper()
	s, e := domain.NewSession("0123456789abcdef0123456789abcdef", "/tmp", "model")
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func navModel(t *testing.T, s *domain.Session) AppModel {
	t.Helper()
	store, e := storage.New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { store.Close() })
	return update(New(Dependencies{Session: s, Store: store, Monochrome: true}), tea.WindowSizeMsg{Width: 70, Height: 20})
}
func drain(t *testing.T, m AppModel, cmd tea.Cmd) AppModel {
	t.Helper()
	for n := 0; cmd != nil && n < 100; n++ {
		next, c := m.Update(cmd())
		m = next.(AppModel)
		cmd = c
	}
	if m.Busy {
		t.Fatal("operation never completed")
	}
	return m
}
func click(t *testing.T, m AppModel, id string) (tea.Model, tea.Cmd) {
	t.Helper()
	m.View()
	z := m.zones.Get(m.prefix + id)
	for deadline := time.Now().Add(time.Second); z == nil && time.Now().Before(deadline); {
		time.Sleep(time.Millisecond)
		z = m.zones.Get(m.prefix + id)
	}
	if z == nil {
		t.Fatalf("missing zone %s", id)
	}
	return m.Update(tea.MouseClickMsg{X: z.StartX, Y: z.StartY, Button: tea.MouseLeft})
}
func TestNavigationPaletteHelpAndInfo(t *testing.T) {
	s := navSession(t)
	m := navModel(t, &s)
	m.Composer.Input.SetValue("keep draft")
	m = update(m, tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	if !strings.Contains(m.View().Content, "Sessions") {
		t.Fatal(m.View().Content)
	}
	m = update(m, tea.KeyPressMsg{Code: 'h', Text: "h"})
	if !strings.Contains(m.View().Content, "Ctrl+S") || !strings.Contains(m.View().Content, "Approve") {
		t.Fatal(m.View().Content)
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	m = update(m, ControlIntent("info"))
	if !strings.Contains(m.View().Content, "/tmp") {
		t.Fatal(m.View().Content)
	}
	if m.Composer.Input.Value() != "keep draft" {
		t.Fatal("overlay changed editor")
	}
}
func TestNavigationSearchHits(t *testing.T) {
	s := navSession(t)
	s.BeginTurn("needle first", nil)
	s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: root.Text(strings.Repeat("line\n", 50) + "needle last")}})
	m := navModel(t, &s)
	m = update(m, ControlIntent("search"))
	for _, r := range "needle" {
		m = update(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if !strings.Contains(m.View().Content, "1/2") {
		t.Fatal(m.View().Content)
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if !strings.Contains(m.View().Content, "2/2") {
		t.Fatal(m.View().Content)
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.Transcript.Viewport.YOffset() == 0 {
		t.Fatal("search did not navigate transcript")
	}
}
func TestNavigationRestoreRequiresExplicitContinue(t *testing.T) {
	s := navSession(t)
	s.BeginTurn("question", nil)
	s.InterruptDraft("partial")
	m := navModel(t, &s)
	m.deps.Agent = application.AgentTurnUseCase{Continue: application.ContinueTurnUseCase{Store: m.deps.Store, Models: navStream{}}}
	if m.Init() != nil || m.Busy {
		t.Fatal("load executed")
	}
	if !strings.Contains(m.View().Content, "Continue") {
		t.Fatal("no explicit continuation")
	}
	n, cmd := m.Update(ControlIntent("continue"))
	m = n.(AppModel)
	if !m.Busy || s.Status() != domain.StatusInterrupted {
		t.Fatal("resume bypassed operation isolation")
	}
	m = drain(t, m, cmd)
	if s.Status() != domain.StatusComplete || !strings.Contains(m.View().Content, "continued answer") {
		t.Fatal(m.View().Content)
	}
}

type navStream struct{}

func (navStream) Stream(_ context.Context, req root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
	return root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "continued answer"}}, nil
}
func TestNavigationSessionPickerLoadsWithoutExecution(t *testing.T) {
	s := navSession(t)
	m := navModel(t, &s)
	other, _ := domain.NewSession("1123456789abcdef0123456789abcdef", "/tmp", "other-model")
	other.BeginTurn("saved question", nil)
	other.InterruptDraft("saved partial")
	if e := m.deps.Store.Save(context.Background(), other); e != nil {
		t.Fatal(e)
	}
	m.Composer.Input.SetValue("keep")
	n, c := m.Update(ControlIntent("sessions"))
	m = drain(t, n.(AppModel), c)
	if !strings.Contains(m.View().Content, "other-model") {
		t.Fatal(m.View().Content)
	}
	n, c = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if s.Export().ID == other.Export().ID {
		t.Fatal("load mutated before completion")
	}
	m = drain(t, n.(AppModel), c)
	if s.Export().ID != other.Export().ID || s.Status() != domain.StatusInterrupted || m.Composer.Input.Value() != "keep" {
		t.Fatal("switch lost state or resumed")
	}
}
func TestNavigationErrorsPreserveInputAndState(t *testing.T) {
	s := navSession(t)
	m := navModel(t, &s)
	m.deps.Store = navBrokenStore{}
	m.Composer.Input.SetValue("keep")
	n, c := m.Update(ControlIntent("sessions"))
	m = drain(t, n.(AppModel), c)
	if m.Status.State != domain.StatusIdle || m.Composer.Input.Value() != "keep" || !strings.Contains(m.View().Content, "storage offline") {
		t.Fatal(m.View().Content)
	}
}

type navBrokenStore struct{}

func (navBrokenStore) List(context.Context) ([]domain.SessionSummary, error) {
	return nil, errors.New("storage offline")
}
func (navBrokenStore) Save(context.Context, domain.Session) error {
	return errors.New("storage offline")
}
func (navBrokenStore) Load(context.Context, domain.SessionID) (domain.Session, error) {
	return domain.Session{}, errors.New("storage offline")
}
func TestNavigationMinimumSizeOverlays(t *testing.T) {
	s := navSession(t)
	m := navModel(t, &s)
	m = update(m, tea.WindowSizeMsg{Width: 50, Height: 15})
	for _, intent := range []ControlIntent{"palette", "help", "info", "search"} {
		m = update(m, intent)
		v := m.View().Content
		if lipgloss.Width(v) > 50 || lipgloss.Height(v) > 15 {
			t.Fatalf("%s overflow: %dx%d", intent, lipgloss.Width(v), lipgloss.Height(v))
		}
	}
}
func TestNavigationSwitchRejectsDifferentWorkspace(t *testing.T) {
	s := navSession(t)
	m := navModel(t, &s)
	other, _ := domain.NewSession("1123456789abcdef0123456789abcdef", "/another-workspace", "model")
	m.deps.Store.Save(context.Background(), other)
	n, c := m.Update(ControlIntent("sessions"))
	m = drain(t, n.(AppModel), c)
	n, c = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = drain(t, n.(AppModel), c)
	if s.Export().Workspace != "/tmp" || !strings.Contains(m.Status.Error, "differs from active workspace") {
		t.Fatal("workspace compatibility not enforced")
	}
}
func TestNavigationSearchDuplicateMessageTargets(t *testing.T) {
	s := navSession(t)
	s.BeginTurn("match", nil)
	s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: root.Text(strings.Repeat("line\n", 50))}})
	s.BeginTurn("match", nil)
	s.InterruptDraft("")
	m := navModel(t, &s)
	m = update(m, ControlIntent("search"))
	for _, r := range "match" {
		m = update(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.Transcript.Viewport.YOffset() < 40 {
		t.Fatal("second identical message navigated to first")
	}
}
func TestNavigationInterruptedMinimumWidth(t *testing.T) {
	s := navSession(t)
	s.BeginTurn("q", nil)
	s.InterruptDraft("draft")
	m := navModel(t, &s)
	m = update(m, tea.WindowSizeMsg{Width: 50, Height: 15})
	v := m.View().Content
	if lipgloss.Width(v) > 50 || lipgloss.Height(v) > 15 {
		t.Fatalf("interrupted controls overflow: %dx%d", lipgloss.Width(v), lipgloss.Height(v))
	}
}
func TestNavigationLongPickerAndInfoFit(t *testing.T) {
	s := navSession(t)
	m := navModel(t, &s)
	m.Header.State.Workspace = domain.Workspace("/tmp/" + strings.Repeat("wide", 100))
	m = update(m, tea.WindowSizeMsg{Width: 50, Height: 15})
	m = update(m, ControlIntent("info"))
	v := m.View().Content
	if lipgloss.Width(v) > 50 || lipgloss.Height(v) > 15 {
		t.Fatal("info overflows")
	}
	m.overlay = "sessions"
	m.Picker.Items = []domain.SessionSummary{{ID: s.Export().ID, Workspace: m.Header.State.Workspace, Model: "model"}}
	v = m.View().Content
	if lipgloss.Width(v) > 50 || lipgloss.Height(v) > 15 {
		t.Fatal("picker overflows")
	}
}
func TestNavigationMousePaletteSearchAndHelp(t *testing.T) {
	s := navSession(t)
	m := navModel(t, &s)
	n, _ := click(t, m, "palette")
	m = n.(AppModel)
	n, _ = click(t, m, "search")
	m = n.(AppModel)
	if !strings.Contains(m.View().Content, "Search:") {
		t.Fatal("mouse search missing")
	}
	n, _ = click(t, m, "close")
	m = n.(AppModel)
	n, _ = click(t, m, "help")
	m = n.(AppModel)
	if !strings.Contains(m.View().Content, "Ctrl+S") {
		t.Fatal("mouse help missing")
	}
}
func TestNavigationSearchCursorAndPasteFocus(t *testing.T) {
	s := navSession(t)
	m := navModel(t, &s)
	m.Composer.Input.SetValue("draft")
	m = update(m, ControlIntent("search"))
	m = update(m, tea.PasteMsg{Content: "query"})
	if m.SearchBox.Input.Value() != "query" || m.Composer.Input.Value() != "draft" {
		t.Fatal("paste missed focused search")
	}
	if m.View().Cursor == nil {
		t.Fatal("search cursor invisible")
	}
}
func TestNavigationHelpQuitMatchesBinding(t *testing.T) {
	s := navSession(t)
	m := navModel(t, &s)
	m = update(m, ControlIntent("help"))
	_, c := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if c == nil {
		t.Fatal("documented quit key swallowed")
	}
}
func TestNavigationSearchResizeKeepsQueryInBounds(t *testing.T) {
	s := navSession(t)
	m := navModel(t, &s)
	m = update(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = update(m, ControlIntent("search"))
	m = update(m, tea.PasteMsg{Content: strings.Repeat("q", 90)})
	m = update(m, tea.WindowSizeMsg{Width: 50, Height: 15})
	v := m.View()
	if lipgloss.Width(v.Content) > 50 || m.SearchBox.Input.Value() != strings.Repeat("q", 90) || v.Cursor.X >= 50 {
		t.Fatalf("resize query=%d width=%d cursor=%+v view=%s", len(m.SearchBox.Input.Value()), lipgloss.Width(v.Content), v.Cursor, v.Content)
	}
}
