package terminal

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type fakeRepairs struct {
	records   []domain.RepairRecord
	decisions []string
	recovered []string
	events    chan application.RepairEvent
	err       error
}

func (f *fakeRepairs) Records(context.Context) ([]domain.RepairRecord, error) {
	return append([]domain.RepairRecord(nil), f.records...), f.err
}
func (f *fakeRepairs) Decide(_ context.Context, id string, approve bool, reason string) error {
	if f.err != nil {
		return f.err
	}
	verdict := "deny"
	if approve {
		verdict = "approve"
	}
	f.decisions = append(f.decisions, id+":"+verdict+":"+reason)
	return nil
}
func (f *fakeRepairs) Recover(_ context.Context, id string) error {
	f.recovered = append(f.recovered, id)
	return f.err
}
func (f *fakeRepairs) Events() <-chan application.RepairEvent { return f.events }

const panelParent = "0123456789abcdef0123456789abcdef"

func repairModel(t *testing.T, repairs *fakeRepairs) AppModel {
	t.Helper()
	session, err := domain.NewSession(panelParent, testWorkspace(), "model")
	if err != nil {
		t.Fatal(err)
	}
	m := New(Dependencies{Session: &session, Monochrome: true, Repairs: repairs})
	return update(m, tea.WindowSizeMsg{Width: 100, Height: 30})
}

func awaitingRecord(status domain.RepairStatus) domain.RepairRecord {
	return domain.RepairRecord{ID: "20261005-1200-edit-fails", Signature: "s", Repository: "o/r", Parent: panelParent, Session: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Status: status, Step: "reproduce", Instance: "axlr-child-1", Pending: "run the check command go test ./... in the clone", Clone: "/tmp/repairs/x", Created: time.Now(), Updated: time.Now()}
}

func TestRepairPanelOpensForADecisionAndApprovesIt(t *testing.T) {
	repairs := &fakeRepairs{records: []domain.RepairRecord{awaitingRecord(domain.RepairAwaitingApproval)}, events: make(chan application.RepairEvent, 1)}
	m := repairModel(t, repairs)
	if m.overlay != "repairs" {
		t.Fatalf("panel not opened for a pending decision: %q", m.overlay)
	}
	view := m.View().Content
	for _, want := range []string{"Self-repairs and improvements", "waiting for your approval", "go test ./...", "axlr-child-1", "a approve"} {
		if !strings.Contains(view, want) {
			t.Fatalf("panel lacks %q:\n%s", want, view)
		}
	}
	m = update(m, tea.KeyPressMsg{Code: 'a', Text: "a"})
	if len(repairs.decisions) != 1 || repairs.decisions[0] != "20261005-1200-edit-fails:approve:" {
		t.Fatalf("decisions %v", repairs.decisions)
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.overlay != "" {
		t.Fatal("esc closes the panel")
	}
	// The same decision does not reopen the panel on its own; a new one does.
	m = update(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	if m.overlay != "" {
		t.Fatal("the panel reopened for the decision the person saw")
	}
	repairs.records[0].Status, repairs.records[0].Pending = domain.RepairAwaitingMerge, "merge pull request #7"
	repairs.events <- application.RepairEvent{Record: repairs.records[0]}
	m = update(m, repairEventMsg(application.RepairEvent{Record: repairs.records[0]}))
	if m.overlay != "repairs" || !strings.Contains(m.View().Content, "merge pull request #7") {
		t.Fatalf("panel not refreshed for the merge decision: %q", m.overlay)
	}
	if badge := m.repairBadge(); !strings.Contains(badge, "repair edit-fails") || !strings.Contains(badge, "merge decision") {
		t.Fatalf("badge %q", badge)
	}
}

func TestRepairPanelDeclinesAMergeWithAReasonAndDeniesACheckDirectly(t *testing.T) {
	repairs := &fakeRepairs{records: []domain.RepairRecord{awaitingRecord(domain.RepairAwaitingMerge)}}
	m := repairModel(t, repairs)
	m = update(m, tea.KeyPressMsg{Code: 'd', Text: "d"})
	if !m.RepairPanel.Reasoning {
		t.Fatal("declining a merge asks for a reason")
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.Status.Error != Translate(English, "repairs.reasonRequired") || len(repairs.decisions) != 0 {
		t.Fatalf("empty reason accepted: %q %v", m.Status.Error, repairs.decisions)
	}
	for _, r := range "no" {
		m = update(m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if len(repairs.decisions) != 1 || repairs.decisions[0] != "20261005-1200-edit-fails:deny:no" || m.RepairPanel.Reasoning {
		t.Fatalf("decline %v", repairs.decisions)
	}
	m = update(m, tea.KeyPressMsg{Code: 'd', Text: "d"})
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.RepairPanel.Reasoning || m.overlay != "repairs" {
		t.Fatal("esc cancels the reason, not the panel")
	}
	repairs.records[0].Status = domain.RepairAwaitingApproval
	m = update(m, repairEventMsg(application.RepairEvent{Record: repairs.records[0]}))
	m = update(m, tea.KeyPressMsg{Code: 'd', Text: "d"})
	if len(repairs.decisions) != 2 || repairs.decisions[1] != "20261005-1200-edit-fails:deny:" {
		t.Fatalf("deny %v", repairs.decisions)
	}
	repairs.err = errors.New("repair is not waiting for a decision")
	m = update(m, tea.KeyPressMsg{Code: 'a', Text: "a"})
	if m.Status.Error != "repair is not waiting for a decision" {
		t.Fatalf("error not shown: %q", m.Status.Error)
	}
}

func TestRepairPanelListsAndRecoversAnInterruptedRepairFromTheSlashCommand(t *testing.T) {
	record := awaitingRecord(domain.RepairInterrupted)
	record.Pending, record.Error, record.PullRequest, record.URL = "", "the console that drove it stopped", 7, "https://example.test/pr/7"
	repairs := &fakeRepairs{records: []domain.RepairRecord{record}}
	m := repairModel(t, repairs)
	if m.overlay != "" {
		t.Fatal("an interrupted repair does not open the panel on its own")
	}
	m.Composer.Input.SetValue("/repair")
	m = update(m, ControlIntent("send"))
	if m.overlay != "repairs" || m.Header.State.Mode == domain.ModeRepair {
		t.Fatalf("/repair must show the repairs, not switch mode: %q", m.overlay)
	}
	view := m.View().Content
	for _, want := range []string{"interrupted", "Pull request #7", "r recovers it", "the console that drove it stopped"} {
		if !strings.Contains(view, want) {
			t.Fatalf("panel lacks %q:\n%s", want, view)
		}
	}
	m = update(m, tea.KeyPressMsg{Code: 'a', Text: "a"})
	if m.Status.Error != Translate(English, "repairs.nothingToDecide") {
		t.Fatalf("approve without a decision: %q", m.Status.Error)
	}
	m = update(m, tea.KeyPressMsg{Code: 'r', Text: "r"})
	if len(repairs.recovered) != 1 || repairs.recovered[0] != record.ID {
		t.Fatalf("recovered %v", repairs.recovered)
	}
	repairs.records[0].Status = domain.RepairCompleted
	repairs.records[0].MergeSHA = "abc123def456"
	m = update(m, repairEventMsg(application.RepairEvent{Record: repairs.records[0]}))
	if view := m.View().Content; !strings.Contains(view, "Merged as abc123def456") || !strings.Contains(view, "does not contain the fix") {
		t.Fatalf("completed view:\n%s", view)
	}
	m = update(m, tea.KeyPressMsg{Code: 'r', Text: "r"})
	if m.Status.Error != Translate(English, "repairs.nothingToRecover") {
		t.Fatalf("recover without an interrupted repair: %q", m.Status.Error)
	}
	if badge := m.repairBadge(); badge != "" {
		t.Fatalf("a merged repair has no badge: %q", badge)
	}
}

func TestRepairPanelIsAbsentWithoutACoordinatorOrRecords(t *testing.T) {
	m := sized()
	m.Composer.Input.SetValue("/repair")
	m = update(m, ControlIntent("send"))
	if m.overlay != "" || m.Status.Error != Translate(English, "mode.unavailable") {
		// Without a coordinator /repair reaches the mode switch, which this
		// storeless model refuses; the panel never opens.
		t.Fatalf("without repairs /repair selects the mode: %q %q", m.overlay, m.Status.Error)
	}
	if m.openRepairPanel().Status.Error != Translate(English, "repairs.unavailable") {
		t.Fatal("no coordinator")
	}
	empty := repairModel(t, &fakeRepairs{events: make(chan application.RepairEvent)})
	if empty.openRepairPanel().Status.Error != Translate(English, "repairs.nothing") || empty.Init() == nil {
		t.Fatal("no records, but the event subscription exists")
	}
	other := awaitingRecord(domain.RepairCompleted)
	other.Parent = "fedcba9876543210fedcba9876543210"
	foreign := repairModel(t, &fakeRepairs{records: []domain.RepairRecord{other}})
	if len(foreign.repairRecords()) != 0 {
		t.Fatal("another session's finished repair is not listed here")
	}
	if shortRepairID("plain") != "plain" || shortRepairID("20261005-1200-"+strings.Repeat("x", 30)) != strings.Repeat("x", 18)+"…" {
		t.Fatal(shortRepairID("20261005-1200-" + strings.Repeat("x", 30)))
	}
	if readRepairEvent(nil) != nil {
		t.Fatal("nil channel")
	}
	closed := make(chan application.RepairEvent)
	close(closed)
	if msg := readRepairEvent(closed)(); msg != nil {
		t.Fatal("closed channel ends the subscription")
	}
}

func TestImprovementsShowTheirKindAndOpenFromImprove(t *testing.T) {
	record := awaitingRecord(domain.RepairRunning)
	record.Improvement = true
	repairs := &fakeRepairs{records: []domain.RepairRecord{record}, events: make(chan application.RepairEvent, 1)}
	m := repairModel(t, repairs)
	m.Composer.Input.SetValue("/improve")
	next, _ := m.Update(ControlIntent("send"))
	m = next.(AppModel)
	if m.overlay != "repairs" || m.Header.State.Mode == domain.ModeImprove {
		t.Fatalf("/improve with a linked improvement: overlay %q mode %q", m.overlay, m.Header.State.Mode)
	}
	if view := m.View().Content; !strings.Contains(view, "Improvement 20261005-1200-edit-fails") {
		t.Fatalf("panel lacks the improvement row:\n%s", view)
	}
	if badge := m.repairBadge(); !strings.HasPrefix(badge, "improvement ") {
		t.Fatalf("badge %q", badge)
	}
}

// The reason input sits under the panel's content: a resize keeps the two
// rows the panel and the incident card leave for it.
func TestTheReasonInputSurvivesAResize(t *testing.T) {
	repairs := &fakeRepairs{records: []domain.RepairRecord{awaitingRecord(domain.RepairAwaitingMerge)}}
	m := repairModel(t, repairs)
	m = update(m, tea.KeyPressMsg{Code: 'd', Text: "d"})
	prompt := strings.TrimSpace(Translate(English, "repairs.reasonPrompt"))
	if !strings.Contains(m.View().Content, prompt) {
		t.Fatal("the reason input is not shown")
	}
	for _, size := range []tea.WindowSizeMsg{{Width: 100, Height: 30}, {Width: 90, Height: 26}} {
		if m = update(m, size); !m.RepairPanel.Reasoning || !strings.Contains(m.View().Content, prompt) {
			t.Fatalf("after a resize to %dx%d the reason input is gone:\n%s", size.Width, size.Height, m.View().Content)
		}
	}

	card := awaitingModel(t, 0)
	card = update(card, tea.KeyPressMsg{Code: 'd', Text: "d"})
	question := strings.TrimSpace(Translate(Spanish, "incident.reasonPrompt"))
	if card = update(card, tea.WindowSizeMsg{Width: 100, Height: 30}); !strings.Contains(card.View().Content, question) {
		t.Fatalf("after a resize the incident card lost its reason input:\n%s", card.View().Content)
	}
}
