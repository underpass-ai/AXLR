package terminal

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"context"
	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
	"path/filepath"
	"strings"
	"testing"
)

func approvalModel(t *testing.T) AppModel {
	t.Helper()
	s := navSession(t)
	args, _ := root.NewJSONValue([]byte(`{"path":"exact-target.txt","content":"all arguments"}`))
	schema, _ := root.NewJSONValue([]byte(`{"type":"object"}`))
	s.BeginTurn("write", []domain.AvailableTool{{Definition: root.ToolDefinition{Name: "local_write", Description: "write", Parameters: schema}, Identity: domain.ToolIdentity{Kind: domain.ToolKindLocal, LocalOperation: "write"}}})
	if e := s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "one", Name: "local_write", Arguments: args}, {ID: "two", Name: "local_write", Arguments: args}}}}); e != nil {
		t.Fatal(e)
	}
	m := navModel(t, &s)
	m.deps.Resolve = application.ResolveToolUseCase{Store: m.deps.Store}
	return m
}
func TestApprovalVisibilityAndFocus(t *testing.T) {
	m := approvalModel(t)
	v := m.View().Content
	for _, want := range []string{"exact-target.txt", "all arguments", "local", "write", string(testWorkspace()), "a approve", "d deny"} {
		if !strings.Contains(v, want) {
			t.Fatalf("missing %s: %s", want, v)
		}
	}
	m.Composer.Input.SetValue("draft")
	for _, msg := range []tea.Msg{ControlIntent("search"), ControlIntent("sessions"), ControlIntent("send"), tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl}, tea.KeyPressMsg{Code: 'x', Text: "x"}} {
		m = update(m, msg)
	}
	if m.Busy || m.Composer.Input.Value() != "draft" || !strings.Contains(m.View().Content, "a approve") {
		t.Fatal("approval lost focus")
	}
}
func TestApprovalDenyKeyboardAndMouse(t *testing.T) {
	for _, mouse := range []bool{false, true} {
		m := approvalModel(t)
		var n tea.Model
		var c tea.Cmd
		if mouse {
			n, c = click(t, m, "deny")
		} else {
			n, c = m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
		}
		m = n.(AppModel)
		if !m.Busy || m.deps.Session.Pending()[0].Call.ID != "one" {
			t.Fatal("decision bypassed isolation")
		}
		m = drain(t, m, c)
		if len(m.deps.Session.Pending()) != 1 || m.deps.Session.Export().Activity[0].Decision != domain.DecisionDeny {
			t.Fatal("deny not recorded")
		}
	}
}
func TestApprovalEscapeCancelsWithoutApproval(t *testing.T) {
	m := approvalModel(t)
	n, c := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = drain(t, n.(AppModel), c)
	if len(m.deps.Session.Pending()) != 0 || m.Status.State != domain.StatusInterrupted {
		t.Fatal("escape did not cancel")
	}
	for _, p := range m.deps.Session.Export().Activity {
		if p.Decision != domain.DecisionDeny || !strings.Contains(string(p.Outcome.Content), "cancelled") {
			t.Fatal("escape approved")
		}
	}
}
func TestApprovalUnknownNeverOffersApproval(t *testing.T) {
	m := approvalModel(t)
	s := navSession(t)
	args, _ := root.NewJSONValue([]byte(`{}`))
	s.BeginTurn("unknown", nil)
	s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "unknown", Name: "missing", Arguments: args}}}})
	m = New(Dependencies{Session: &s, Store: m.deps.Store, Agent: application.AgentTurnUseCase{Continue: application.ContinueTurnUseCase{Store: m.deps.Store, Models: navStream{}}}})
	m = update(m, tea.WindowSizeMsg{Width: 70, Height: 20})
	if strings.Contains(m.View().Content, "a approve") {
		t.Fatal("unknown tool can be approved")
	}
	n, c := m.Update(ControlIntent("continue"))
	m = drain(t, n.(AppModel), c)
	if s.Status() != domain.StatusComplete || s.Export().Activity[0].Decision != domain.DecisionDeny {
		t.Fatal("unknown was not rejected through continuation")
	}
}

func TestApprovalApproveKeyboardAndMouse(t *testing.T) {
	for _, mouse := range []bool{false, true} {
		m := approvalModel(t)
		m.deps.Resolve.Tools = navTool{}
		var n tea.Model
		var c tea.Cmd
		if mouse {
			n, c = click(t, m, "approve")
		} else {
			n, c = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
		}
		m = drain(t, n.(AppModel), c)
		p := m.deps.Session.Export().Activity[0]
		if p.Decision != domain.DecisionApprove || p.Outcome == nil || p.Outcome.Content != "written" || len(m.deps.Session.Pending()) != 1 || !strings.Contains(m.View().Content, "a approve") {
			t.Fatal("approval not executed and correlated")
		}
	}
}

func TestApprovalAlwaysAllowPersistsAndApprovesFollowingCall(t *testing.T) {
	m := approvalModel(t)
	path := filepath.Join(t.TempDir(), "approvals.json")
	settings, err := storage.NewApprovalSettings(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	m.deps.ApprovalSettings = settings
	m.deps.Resolve.Approval = settings
	m.deps.Resolve.Tools = navTool{}
	m.deps.Resolve.Continue.Models = navStream{}
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	m = drain(t, next.(AppModel), cmd)
	identity, _ := domain.NewLocalToolIdentity("write")
	activity := m.deps.Session.Export().Activity
	if !settings.AutoApproves(identity) || len(activity) != 2 || activity[0].Decision != domain.DecisionApprove || activity[1].Decision != domain.DecisionAutoApprove {
		t.Fatalf("always allow did not apply to next call: %+v", activity)
	}
	reloaded, err := storage.NewApprovalSettings(path, nil)
	if err != nil || !reloaded.AutoApproves(identity) {
		t.Fatal("always allow did not persist")
	}
}

func TestApprovalFullAutonomyKeyRunsPendingCalls(t *testing.T) {
	m := approvalModel(t)
	settings, err := storage.NewApprovalSettings(filepath.Join(t.TempDir(), "approvals.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	m.deps.ApprovalSettings = settings
	m.deps.Resolve.Approval = settings
	m.deps.Resolve.Tools = navTool{}
	m.deps.Resolve.Continue.Models = navStream{}
	next, cmd := m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	m = drain(t, next.(AppModel), cmd)
	activity := m.deps.Session.Export().Activity
	if !settings.Autonomous() || !m.Status.Autonomous || len(activity) != 2 || activity[0].Decision != domain.DecisionApprove || activity[1].Decision != domain.DecisionAutoApprove {
		t.Fatalf("full autonomy did not run pending calls: %+v", activity)
	}
}

func TestApprovalsCommandShowsOnlySavedChoices(t *testing.T) {
	m := sized()
	settings, err := storage.NewApprovalSettings(filepath.Join(t.TempDir(), "approvals.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	identity, _ := domain.NewLocalToolIdentity("write")
	if err := settings.Allow(context.Background(), identity); err != nil {
		t.Fatal(err)
	}
	m.deps.ApprovalSettings = settings
	m.Composer.Input.SetValue("/approvals")
	m = update(m, ControlIntent("send"))
	if m.overlay != "approvals" || !strings.Contains(m.View().Content, "write") || m.Composer.Input.Value() != "" {
		t.Fatal("saved approval list did not open")
	}
}
func TestApprovalUnknownKeyboardContinuation(t *testing.T) {
	m := approvalModel(t)
	s := navSession(t)
	args, _ := root.NewJSONValue([]byte(`{}`))
	s.BeginTurn("unknown", nil)
	s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "unknown", Name: "missing", Arguments: args}}}})
	m = update(New(Dependencies{Session: &s, Store: m.deps.Store, Agent: application.AgentTurnUseCase{Continue: application.ContinueTurnUseCase{Store: m.deps.Store, Models: navStream{}}}}), tea.WindowSizeMsg{Width: 50, Height: 15})
	n, c := m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m = drain(t, n.(AppModel), c)
	if s.Status() != domain.StatusComplete {
		t.Fatal("advertised unknown continuation key did nothing")
	}
}
func TestApprovalRefreshesSameCallIDAfterSwitch(t *testing.T) {
	m := approvalModel(t)
	s := navSession(t)
	args, _ := root.NewJSONValue([]byte(`{"path":"different-target"}`))
	s.BeginTurn("another", m.Header.State.ToolSnapshot)
	s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "one", Name: "local_write", Arguments: args}}}})
	m = publishTestSession(t, m, s)
	if !strings.Contains(m.View().Content, "different-target") {
		t.Fatal("approval shows obsolete arguments")
	}
}
func TestApprovalDeferredUnknownAfterKnown(t *testing.T) {
	m := approvalModel(t)
	s := navSession(t)
	args, _ := root.NewJSONValue([]byte(`{}`))
	s.BeginTurn("mixed", m.Header.State.ToolSnapshot)
	s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "known", Name: "local_write", Arguments: args}, {ID: "unknown", Name: "missing", Arguments: args}}}})
	m = publishTestSession(t, m, s)
	m.deps.Resolve.Continue.Models = navStream{}
	n, c := m.Update(ControlIntent("deny"))
	m = drain(t, n.(AppModel), c)
	state := m.Header.State
	if state.Status != domain.StatusComplete || len(state.Activity) != 2 || state.Activity[1].Decision != domain.DecisionDeny || !strings.Contains(string(state.Activity[1].Outcome.Content), "unknown tool") {
		t.Fatal("deferred unknown not rejected")
	}
}

type navTool struct{}

func (navTool) Execute(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error) {
	return domain.ToolOutcome{Content: "written"}, nil
}
func TestApprovalLongArgumentsRemainScrollableWithFixedControls(t *testing.T) {
	m := approvalModel(t)
	s := navSession(t)
	args, _ := root.NewJSONValue([]byte(`{"path":"exact-target.txt","content":"` + strings.Repeat("long-data", 300) + `END-MARKER"}`))
	s.BeginTurn("long", m.Header.State.ToolSnapshot)
	s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "long", Name: "local_write", Arguments: args}}}})
	m = publishTestSession(t, m, s)
	m = update(m, tea.WindowSizeMsg{Width: 50, Height: 15})
	for i := 0; i < 100; i++ {
		m = update(m, tea.KeyPressMsg{Code: tea.KeyPgDown})
	}
	v := m.View().Content
	if !strings.Contains(v, "END-MARKER") || !strings.Contains(v, "a approve") || lipgloss.Width(v) > 50 || lipgloss.Height(v) > 15 {
		t.Fatalf("details or controls not accessible (%dx%d):\n%s", lipgloss.Width(v), lipgloss.Height(v), ansi.Strip(v))
	}
}
func TestApprovalRestoredPendingNeedsResumeBeforeDecision(t *testing.T) {
	m := approvalModel(t)
	restored, e := domain.RestoreSession(m.Header.State)
	if e != nil {
		t.Fatal(e)
	}
	m = publishTestSession(t, m, restored)
	m.deps.Agent = application.AgentTurnUseCase{Continue: application.ContinueTurnUseCase{Store: m.deps.Store}}
	if strings.Contains(m.View().Content, "a approve") {
		t.Fatal("restore reopened approval")
	}
	n, c := m.Update(ControlIntent("continue"))
	m = drain(t, n.(AppModel), c)
	if m.Status.State != domain.StatusApproval || !strings.Contains(m.View().Content, "a approve") || m.Header.State.Activity[0].Decision != "" {
		t.Fatal("resume did not only reopen approvals")
	}
}
func TestApprovalSaveFailurePreservesDecisionAndInput(t *testing.T) {
	m := approvalModel(t)
	m.deps.Resolve.Store = navBrokenStore{}
	m.Composer.Input.SetValue("draft")
	n, c := m.Update(ControlIntent("deny"))
	m = drain(t, n.(AppModel), c)
	if m.Status.State != domain.StatusApproval || len(m.deps.Session.Pending()) != 2 || m.Composer.Input.Value() != "draft" || !strings.Contains(m.Status.Error, "storage offline") {
		t.Fatal("failed decision lost state")
	}
}
func TestApprovalPasteDoesNotReachEditor(t *testing.T) {
	m := approvalModel(t)
	m.Composer.Input.SetValue("draft")
	m = update(m, tea.PasteMsg{Content: "unrelated"})
	if m.Composer.Input.Value() != "draft" {
		t.Fatal("approval focus leaked paste")
	}
}
func TestApprovalIdentityChangeRefreshesExactTarget(t *testing.T) {
	m := approvalModel(t)
	state := m.Header.State
	state.ToolSnapshot[0].Identity = domain.ToolIdentity{Kind: domain.ToolKindPlugin, Plugin: root.PluginRef{PluginID: "server", ToolName: "remote_write"}}
	s, e := domain.RestoreSession(state)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.ResumePending(); e != nil {
		t.Fatal(e)
	}
	m = publishTestSession(t, m, s)
	if !strings.Contains(m.View().Content, "plugin server / remote_write") {
		t.Fatal("approval showed previous tool identity")
	}
}
func TestApprovalComponentEmitsTypedDecision(t *testing.T) {
	m := approvalModel(t)
	for _, tc := range []struct {
		msg  tea.Msg
		want domain.ToolDecision
	}{{tea.KeyPressMsg{Code: 'a', Text: "a"}, domain.DecisionApprove}, {ControlIntent("deny"), domain.DecisionDeny}, {tea.KeyPressMsg{Code: tea.KeyEsc}, ""}} {
		if got := m.Approval.Intent(tc.msg); got != tc.want {
			t.Fatalf("decision = %q", got)
		}
	}
}

// The worker has not completed while follow-up deltas are being displayed.
func TestApprovalFollowupVisibleBeforeOperationComplete(t *testing.T) {
	for _, decision := range []ControlIntent{"approve", "deny"} {
		t.Run(string(decision), func(t *testing.T) {
			m := approvalModel(t)
			// Resolve the first call so the tested decision is the last in its batch.
			n, cmd := m.Update(ControlIntent("deny"))
			m = drain(t, n.(AppModel), cmd)
			release := make(chan struct{})
			defer close(release)
			m.deps.Resolve.Tools = navTool{}
			m.deps.Resolve.Continue.Models = heldFollowup{release: release}
			n, cmd = m.Update(decision)
			m = n.(AppModel)
			for i := 0; i < 10; i++ {
				msg := cmd()
				for {
					batch, ok := msg.(tea.BatchMsg)
					if !ok {
						break
					}
					msg = batch[0]()
				}
				if _, done := msg.(operationComplete); done {
					t.Fatal("completed before follow-up delta")
				}
				n, cmd = m.Update(msg)
				m = n.(AppModel)
				if hasTextDelta(msg) {
					if !m.Busy || !strings.Contains(m.View().Content, "visible follow-up") {
						// Release blocked provider via cancellation even on failure.
						m.cancel()
						t.Fatalf("follow-up hidden during operation: %s", m.View().Content)
					}
					// Esc remains usable while the provider is blocked.
					n, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
					m = drain(t, n.(AppModel), cmd)
					if m.Status.State != domain.StatusInterrupted {
						t.Fatal("stream cancellation unavailable")
					}
					return
				}
			}
			t.Fatal("no follow-up delta")
		})
	}
}

func hasTextDelta(msg tea.Msg) bool {
	if event, ok := msg.(application.Event); ok {
		return event.Kind == application.EventTextDelta
	}
	if batch, ok := msg.(operationBatch); ok {
		for _, item := range batch.Messages {
			if hasTextDelta(item) {
				return true
			}
		}
	}
	return false
}

type heldFollowup struct{ release <-chan struct{} }

func (s heldFollowup) Stream(ctx context.Context, _ root.CompletionRequest, emit func(root.Text) error) (root.CompletionResult, error) {
	if err := emit("visible follow-up"); err != nil {
		return root.CompletionResult{}, err
	}
	select {
	case <-ctx.Done():
		return root.CompletionResult{}, ctx.Err()
	case <-s.release:
		return root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "visible follow-up"}}, nil
	}
}

func TestApprovalReappearsForFollowupCall(t *testing.T) {
	m := approvalModel(t)
	n, cmd := m.Update(ControlIntent("deny"))
	m = drain(t, n.(AppModel), cmd)
	m.deps.Resolve.Continue.Models = followupCall{}
	n, cmd = m.Update(ControlIntent("deny"))
	m = drain(t, n.(AppModel), cmd)
	if !m.approvalFocus() || !strings.Contains(m.View().Content, "new-target.txt") || m.Approval.Pending.Call.ID != "next" {
		t.Fatalf("follow-up approval missing: %s", m.View().Content)
	}
}

type followupCall struct{}

func (followupCall) Stream(_ context.Context, _ root.CompletionRequest, emit func(root.Text) error) (root.CompletionResult, error) {
	if err := emit("next call"); err != nil {
		return root.CompletionResult{}, err
	}
	args, _ := root.NewJSONObject([]byte(`{"path":"new-target.txt","content":"next"}`))
	return root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "next call", ToolCalls: []root.ToolCall{{ID: "next", Name: "local_write", Arguments: args}}}}, nil
}

// Approval fixtures publish through the same operation boundary as real workers.
func publishTestSession(t *testing.T, m AppModel, session domain.Session) AppModel {
	t.Helper()
	cmd := m.BeginOperation(func(_ context.Context, snapshot *domain.Session, _ func(application.Event) error) error {
		*snapshot = session
		return nil
	})
	return drain(t, m, cmd)
}
