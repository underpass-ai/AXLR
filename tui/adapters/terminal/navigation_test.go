package terminal

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"context"
	"errors"
	"fmt"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone/v2"
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
	s, e := domain.NewSession("0123456789abcdef0123456789abcdef", testWorkspace(), "model")
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
		message := cmd()
		// Bubble Tea dispatches a BatchMsg before calling Update. This helper
		// follows the operation reader; clock ticks have separate tests.
		for {
			batch, ok := message.(tea.BatchMsg)
			if !ok {
				break
			}
			message = batch[0]()
		}
		next, c := m.Update(message)
		m = next.(AppModel)
		cmd = c
	}
	if m.Busy {
		t.Fatal("operation never completed")
	}
	return m
}

// syncZones runs render and returns once zones holds the zones of that
// render and nothing else, so a Get after it reads the rendered view.
//
// bubblezone's Scan does not store zones itself. It queues each zone, then a
// clear for every other render, through a FIFO channel to one worker
// goroutine, and Get returns whatever the worker has applied so far. Until
// 24 October 2026 click rendered, then polled Get for up to 5 s until the id
// had a zone. A zone of the same id from an earlier render passes that check
// with the earlier coordinates: in TestNavigationMousePaletteSearchAndHelp
// the "close" lookup after the search opened returned the palette's close
// button at (2,18) from the render before; by the time Update dispatched the
// click the worker had reached the search footer, whose close is at (25,19);
// the click hit nothing, search stayed open, and the search footer has no
// help hint, so "missing zone help" followed after the full 5 s, on
// windows-11-arm and macos-15 on 23–24 October 2026. Rendering again does
// not help: the worker can still be behind. Only the manager's own order of
// events tells a test when a render has been applied, through its public API:
//
//  1. Scan a probe zone and wait until Get sees it. The worker has applied
//     every earlier render.
//  2. Wait for the wall clock to move past the probe's scan. Scan tags its
//     zones with time.Now().Nanosecond(); on Windows that clock advances in
//     steps of up to 15.6 ms, two scans in one step share a tag, and the
//     second one's clear would keep the probe.
//  3. Render. Scan queues this render's zones, then a clear that removes the
//     probe and anything older.
//  4. Wait until the probe is gone. The worker has applied this render's
//     zones and nothing newer exists.
//
// Each wait ends when the worker acts, however slow the machine; only a
// worker that is not running (after Close) hits zoneWorkerGuard, and then
// the test fails instead of hanging.
func syncZones(t *testing.T, zones *zone.Manager, prefix string, render func()) {
	t.Helper()
	probe := prefix + "probe"
	zones.Scan(zones.Mark(probe, "x"))
	scanned := time.Now()
	waitZone(t, zones, probe, true, "the probe zone was never registered")
	for time.Now().UnixNano() <= scanned.UnixNano() {
		time.Sleep(50 * time.Microsecond)
	}
	render()
	waitZone(t, zones, probe, false, "the rendered view's zones were never applied")
}

// zoneWorkerGuard bounds a wait on bubblezone's worker. It is not a speed
// budget: the waits in syncZones end as soon as the worker acts, and only a
// worker that is not running at all can take this long.
const zoneWorkerGuard = 30 * time.Second

func waitZone(t *testing.T, zones *zone.Manager, id string, present bool, failure string) {
	t.Helper()
	deadline := time.Now().Add(zoneWorkerGuard)
	for (zones.Get(id) != nil) != present {
		if time.Now().After(deadline) {
			t.Fatalf("%s within %s: the zone manager's worker is not running", failure, zoneWorkerGuard)
		}
		time.Sleep(50 * time.Microsecond)
	}
}

// renderedZone renders m and returns zone id of that render. A nil zone after
// syncZones means the rendered view has no such control, which fails at once.
func renderedZone(t *testing.T, m AppModel, id string) *zone.ZoneInfo {
	t.Helper()
	syncZones(t, m.zones, m.prefix, func() { m.View() })
	z := m.zones.Get(m.prefix + id)
	if z == nil {
		t.Fatalf("zone %s is not in the rendered view:\n%s", id, m.View().Content)
	}
	return z
}

// click clicks zone id in m's rendered view.
func click(t *testing.T, m AppModel, id string) (tea.Model, tea.Cmd) {
	t.Helper()
	z := renderedZone(t, m, id)
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
	if !strings.Contains(m.View().Content, "Shift+Enter") {
		t.Fatal(m.View().Content)
	}
	// At 20 rows the help scrolls to its last rows.
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEnd})
	if !strings.Contains(m.View().Content, "Approvals and catalogs") {
		t.Fatal(m.View().Content)
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	m = update(m, ControlIntent("info"))
	if !strings.Contains(m.View().Content, string(testWorkspace())) {
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
	// Init only saves the terminal's title before setting it, and arms the
	// tick that ends the footer's notices (not run here: it waits).
	if msg := m.Init()(); m.Busy || msg == nil {
		t.Fatal("load executed")
	} else if batch, ok := msg.(tea.BatchMsg); !ok || len(batch) != 2 {
		t.Fatalf("init does more than save the title and arm the notice tick: %T", msg)
	} else if _, nested := batch[0]().(tea.BatchMsg); nested || m.Busy {
		t.Fatal("init does more than save the title")
	}
	if !strings.Contains(m.View().Content, "ctrl+r continue") {
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
	other, _ := domain.NewSession("1123456789abcdef0123456789abcdef", testWorkspace(), "other-model")
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
	other, _ := domain.NewSession("1123456789abcdef0123456789abcdef", domain.Workspace(string(testWorkspace())+"-other"), "model")
	other.BeginTurn("elsewhere", nil)
	m.deps.Store.Save(context.Background(), other)
	n, c := m.Update(ControlIntent("sessions"))
	m = drain(t, n.(AppModel), c)
	if len(m.Picker.Visible()) != 0 {
		t.Fatal("another workspace's session is listed by default")
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if current, ok := m.Picker.Current(); !ok || current.ID != other.Export().ID {
		t.Fatal("Tab did not show every workspace")
	}
	// The row says it belongs elsewhere, and Enter says how to open it
	// without starting a load that fails.
	if view := ansi.Strip(m.View().Content); !strings.Contains(view, m.Theme.Tf("sessions.elsewhere", string(testWorkspace())+"-other")) {
		t.Fatalf("no other-workspace marker:\n%s", view)
	}
	n, c = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if c != nil || n.(AppModel).Busy {
		t.Fatal("Enter on another workspace's session started an operation")
	}
	m = n.(AppModel)
	if s.Export().Workspace != testWorkspace() || !strings.Contains(m.Status.Error, "axlr-tui --root "+string(testWorkspace())+"-other --session "+string(other.Export().ID)) || m.overlay != "sessions" {
		t.Fatalf("workspace compatibility not enforced or not explained: %q", m.Status.Error)
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

func TestNavigationSearchTargetsArchivedDraftRow(t *testing.T) {
	s := navSession(t)
	// Nineteen lines, so the prompt is not folded, that wrap to forty.
	if err := s.BeginTurn(root.Text(strings.Repeat("earlier line with enough words to wrap twice at seventy columns of width\n", 19)), nil); err != nil {
		t.Fatal(err)
	}
	if err := s.InterruptDraft("archived needle"); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn("later question", nil); err != nil {
		t.Fatal(err)
	}
	m := navModel(t, &s)
	m = update(m, ControlIntent("search"))
	for _, r := range "needle" {
		m = update(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	if len(m.SearchBox.Hits) != 1 {
		t.Fatalf("search missed archived draft: %+v", m.SearchBox.Hits)
	}
	if m.Transcript.Viewport.YOffset() == 0 || !strings.Contains(m.Transcript.View(), "archived needle") {
		t.Fatalf("search did not reveal archived row at offset %d: %q", m.Transcript.Viewport.YOffset(), m.Transcript.View())
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
	if !strings.Contains(m.View().Content, "Shift+Enter") {
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
	m = update(m, tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if m.overlay != "help" || !strings.Contains(ansi.Strip(m.View().Content), "press Ctrl+C again to quit") {
		t.Fatalf("the first Ctrl+C under help does not ask for a second one:\n%s", ansi.Strip(m.View().Content))
	}
	_, c := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if c == nil {
		t.Fatal("documented quit key swallowed")
	}
	if _, ok := c().(tea.QuitMsg); !ok {
		t.Fatal("a second Ctrl+C under help did not quit")
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

// On 12 October 2026 Enter on the open session's row in Ctrl+O left the
// picker open with no message while the session reloaded behind it.
func TestEnterOnTheOpenSessionClosesThePickerWithoutReloading(t *testing.T) {
	s := navSession(t)
	if err := s.BeginTurn("saved prompt", nil); err != nil {
		t.Fatal(err)
	}
	m := navModel(t, &s)
	if err := m.deps.Store.Save(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	n, cmd := m.Update(ControlIntent("sessions"))
	m = drain(t, n.(AppModel), cmd)
	if m.overlay != "sessions" {
		t.Fatalf("picker not open: %q", m.overlay)
	}
	if selected, ok := m.Picker.Current(); !ok || selected.ID != m.Header.State.ID {
		t.Fatal("the open session is not the selected row")
	}
	n, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = n.(AppModel)
	if cmd != nil || m.Busy {
		t.Fatal("Enter on the open session reloaded it")
	}
	if m.overlay != "" {
		t.Fatalf("the picker stayed open: %q", m.overlay)
	}
}

// On 12 October 2026 a session opened from Ctrl+O kept the offset the
// previous one was scrolled to (row 5 of 159) instead of its last message.
func TestOpeningAnotherSessionShowsItsLastMessage(t *testing.T) {
	long := func(id domain.SessionID, prompt string) domain.Session {
		s, err := domain.NewSession(id, testWorkspace(), "model")
		if err != nil {
			t.Fatal(err)
		}
		for i := range 30 {
			if err := s.BeginTurn(root.Text(fmt.Sprintf("%s %d", prompt, i)), nil); err != nil {
				t.Fatal(err)
			}
			if err := s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "reply\nwith\nlines"}}); err != nil {
				t.Fatal(err)
			}
		}
		return s
	}
	a := long("0123456789abcdef0123456789abcdef", "first session")
	b := long("fedcba9876543210fedcba9876543210", "second session")
	m := navModel(t, &a)
	for _, s := range []domain.Session{a, b} {
		if err := m.deps.Store.Save(context.Background(), s); err != nil {
			t.Fatal(err)
		}
	}
	m.Transcript.Viewport.GotoTop()
	n, cmd := m.Update(ControlIntent("sessions"))
	m = drain(t, n.(AppModel), cmd)
	for i, row := range m.Picker.Visible() {
		if row.ID == b.Export().ID {
			m.Picker.Selected = i
		}
	}
	n, cmd = m.Update(ControlIntent("open-session"))
	m = drain(t, n.(AppModel), cmd)
	if m.Header.State.ID != b.Export().ID {
		t.Fatal("the other session did not open")
	}
	if m.Transcript.Viewport.YOffset() == 0 || !m.Transcript.Viewport.AtBottom() {
		t.Fatalf("the opened session is not at its last message: offset %d", m.Transcript.Viewport.YOffset())
	}
}

// On 12 October 2026 F1 at 15 rows lost its last rows (approve, quit,
// copy) and could not scroll; F1 again did not close it.
func TestHelpScrollsWhenItDoesNotFitAndF1ClosesIt(t *testing.T) {
	s := navSession(t)
	m := navModel(t, &s)
	m = update(m, tea.WindowSizeMsg{Width: 80, Height: 15})
	m = update(m, tea.KeyPressMsg{Code: tea.KeyF1})
	if m.overlay != "help" {
		t.Fatalf("F1 did not open the help: %q", m.overlay)
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "↓") || !strings.Contains(view, "more") || strings.Contains(view, "Ctrl+C twice") {
		t.Fatalf("the help does not say more rows follow:\n%s", view)
	}
	if lipgloss.Height(m.View().Content) > 15 {
		t.Fatal("the help is taller than the screen")
	}
	for range 40 {
		m = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	view = ansi.Strip(m.View().Content)
	if !strings.Contains(view, "/theme: I icons") || !strings.Contains(view, "above") || strings.Contains(view, "more") {
		t.Fatalf("↓ did not reach the last rows:\n%s", view)
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyPgUp})
	m = update(m, tea.KeyPressMsg{Code: tea.KeyHome})
	if m.Help.Top != 0 {
		t.Fatalf("Home left the help at row %d", m.Help.Top)
	}
	m = update(m, tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if m.Help.Top != 3 {
		t.Fatalf("the wheel moved the help to row %d", m.Help.Top)
	}
	m = update(m, tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if m.Help.Top != 0 {
		t.Fatalf("the wheel moved the help back to row %d", m.Help.Top)
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyF1})
	if m.overlay != "" {
		t.Fatalf("F1 did not close the help: %q", m.overlay)
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyF1})
	if m.overlay != "help" || m.Help.Top != 0 {
		t.Fatalf("the help reopened at row %d", m.Help.Top)
	}
}
