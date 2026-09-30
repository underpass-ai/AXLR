package terminal

import (
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"context"
	"errors"
	"fmt"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
	"strings"
	"testing"
)

func TestModelSelectionBareScreenBlocksSend(t *testing.T) {
	m := bareModel()
	defer m.Close()
	if !strings.Contains(m.View().Content, "Type /model to choose a model") {
		t.Error("bare screen lacks selection guidance")
	}
	m.Composer.Input.SetValue("keep my draft")
	n, cmd := m.Update(ControlIntent("send"))
	m = n.(AppModel)
	if cmd != nil || m.Busy || m.Composer.Input.Value() != "keep my draft" || len(m.Header.State.Messages) != 0 {
		t.Fatal("unconfigured send consumed draft or started a turn")
	}
}
func TestSlashModelOpensWithoutHistory(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{{Code: tea.KeyEnter}} {
		m := bareModel()
		m.Composer.Input.SetValue("/model")
		n, cmd := m.Update(key)
		m = n.(AppModel)
		if cmd == nil || !strings.Contains(m.View().Content, "Loading models") || len(m.Header.State.Messages) != 0 || m.Composer.Input.Value() != "" {
			t.Error("slash model failed to open discovery without a user message")
		}
		m.Close()
	}
}

func bareModel() AppModel {
	return update(New(Dependencies{Monochrome: true}), tea.WindowSizeMsg{Width: 100, Height: 30})
}
func selectionModel(t *testing.T, session *domain.Session) (AppModel, *storage.SessionStore) {
	t.Helper()
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	preference, err := storage.NewModelPreferenceStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := update(New(Dependencies{Session: session, Workspace: testWorkspace(), NewSessionID: "0123456789abcdef0123456789abcdef", Store: store, Models: application.ListModelsUseCase{Catalog: modelCatalogStub{}}, ModelPreference: preference, Create: application.CreateSessionUseCase{Store: store}, Change: application.ChangeSessionModelUseCase{Store: store}, Monochrome: true}), tea.WindowSizeMsg{Width: 100, Height: 30})
	t.Cleanup(m.Close)
	return m, store
}
func openModels(t *testing.T, m AppModel) AppModel {
	t.Helper()
	n, cmd := m.Update(ControlIntent("models"))
	if cmd == nil {
		t.Fatal("catalog operation absent")
	}
	return drain(t, n.(AppModel), cmd)
}
func chooseModel(t *testing.T, m AppModel) AppModel {
	t.Helper()
	n, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("selection operation absent")
	}
	return drain(t, n.(AppModel), cmd)
}

func TestModelSelectionCreateAndChangePersist(t *testing.T) {
	for _, configured := range []bool{false, true} {
		t.Run(map[bool]string{false: "create", true: "change"}[configured], func(t *testing.T) {
			var session *domain.Session
			if configured {
				s := navSession(t)
				session = &s
				s.BeginTurn("earlier", nil)
				s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, Content: "answer"}})
			}
			m, store := selectionModel(t, session)
			m.Composer.Input.SetValue("draft survives")
			before, _ := store.List(context.Background())
			if len(before) != 0 {
				t.Fatal("startup persisted session")
			}
			if configured && (!strings.Contains(m.View().Content, "model") || m.overlay != "") {
				t.Fatal("direct session did not open normally")
			}
			m = openModels(t, m)
			if !strings.Contains(m.View().Content, "provider/chosen") {
				t.Fatalf("catalog not rendered: %q", m.View().Content)
			}
			m = chooseModel(t, m)
			if m.Header.State.Model != "provider/chosen" || m.overlay != "" || m.Composer.Input.Value() != "draft survives" {
				t.Fatal("selection did not publish or lost draft")
			}
			preferred, err := m.deps.ModelPreference.Load(context.Background())
			if err != nil || preferred != "provider/chosen" {
				t.Fatalf("selected model not remembered: %q, %v", preferred, err)
			}
			saved, err := store.Load(context.Background(), "0123456789abcdef0123456789abcdef")
			if err != nil || saved.Export().Model != "provider/chosen" {
				t.Fatalf("not persisted: %v", err)
			}
			want := 0
			if configured {
				want = 2
			}
			if len(saved.Messages()) != want {
				t.Fatal("selection changed transcript")
			}
			m.deps.Start = application.StartTurnUseCase{Catalog: submissionCatalog{err: errors.New("offline")}, Store: store}
			n, cmd := m.Update(ControlIntent("send"))
			m = drain(t, n.(AppModel), cmd)
			if !strings.Contains(m.Status.Error, "offline") || m.Composer.Input.Value() != "draft survives" {
				t.Fatal("selected session could not enter normal prompt path")
			}
		})
	}
}
func TestModelSelectionFailureRetryAndFocus(t *testing.T) {
	m, store := selectionModel(t, nil)
	m.deps.Models.Catalog = modelCatalogStub{err: errors.New("unavailable")}
	m.Composer.Input.SetValue("draft")
	m = update(m, ControlIntent("palette"))
	n, cmd := m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	m = n.(AppModel)
	if !strings.Contains(m.View().Content, "Loading models") {
		t.Fatal("no loading screen")
	}
	m = drain(t, m, cmd)
	if !strings.Contains(m.View().Content, "unavailable") || !strings.Contains(m.View().Content, "Retry") {
		t.Fatalf("no retry error: %q", m.View().Content)
	}
	rows, _ := store.List(context.Background())
	if len(rows) != 0 {
		t.Fatal("failed discovery persisted")
	}
	m.deps.Models.Catalog = modelCatalogStub{}
	n, cmd = m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = drain(t, n.(AppModel), cmd)
	m = update(m, tea.PasteMsg{Content: "chosen"})
	m = update(m, tea.WindowSizeMsg{Width: 65, Height: 20})
	if m.Models.Input.Value() != "chosen" || m.Composer.Input.Value() != "draft" || m.View().Cursor == nil {
		t.Fatal("picker did not own focus or resize lost query")
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.overlay != "" || m.Composer.Input.Value() != "draft" {
		t.Fatal("close lost draft")
	}
}
func TestModelSelectionSaveFailureKeepsOldModel(t *testing.T) {
	s := navSession(t)
	m, _ := selectionModel(t, &s)
	m = openModels(t, m)
	m.deps.Change.Store = submissionStore{err: errors.New("disk full")}
	m = chooseModel(t, m)
	if s.Export().Model != "model" || m.Header.State.Model != "model" || !strings.Contains(m.View().Content, "disk full") {
		t.Fatal("failed save published selection")
	}
}

type unavailableModelPreference struct{}

func (unavailableModelPreference) Load(context.Context) (root.ModelID, error) { return "", nil }
func (unavailableModelPreference) Save(context.Context, root.ModelID) error {
	return errors.New("preference disk full")
}

func TestModelSelectionKeepsChosenSessionWhenDefaultCannotBeSaved(t *testing.T) {
	m, _ := selectionModel(t, nil)
	m.deps.ModelPreference = unavailableModelPreference{}
	m = chooseModel(t, openModels(t, m))
	if m.Header.State.Model != "provider/chosen" || m.overlay != "" || !strings.Contains(m.Status.Error, "default") {
		t.Fatalf("selection and preference failure were conflated: %+v", m.Status)
	}
}
func TestModelSelectionBusyAndPendingPriority(t *testing.T) {
	s := navSession(t)
	m, _ := selectionModel(t, &s)
	m.Composer.Input.SetValue("/model")
	m.Busy = true
	n, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = n.(AppModel)
	if cmd != nil || m.Composer.Input.Value() != "/model" || m.Status.Error == "" {
		t.Fatal("busy model command not rejected with draft")
	}
	m.Busy = false
	s.BeginTurn("question", nil)
	args, _ := root.NewJSONObject([]byte(`{}`))
	s.CompleteAssistant(root.CompletionResult{Message: root.Message{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "call", Name: "read", Arguments: args}}}})
	m.Header.State = s.Export()
	m.syncApproval()
	n, cmd = m.Update(ControlIntent("models"))
	m = n.(AppModel)
	if cmd != nil || m.overlay == "models" || !m.approvalFocus() {
		t.Fatal("picker bypassed approval")
	}
}
func TestModelSelectionStaleCompletionIgnored(t *testing.T) {
	m, _ := selectionModel(t, nil)
	n, cmd := m.Update(ControlIntent("models"))
	m = n.(AppModel)
	done := cmd().(operationComplete)
	m = update(m, done)
	m = update(m, ControlIntent(ModelCloseIntent))
	n, next := m.Update(ControlIntent("models"))
	m = n.(AppModel)
	stale := navSession(t)
	done.Session = stale
	done.Err = errors.New("stale error")
	m = update(m, done)
	if !m.Busy || m.Header.State.ID != "" || m.Status.Error != "" {
		t.Fatal("stale completion changed current operation")
	}
	m = drain(t, m, next)
}

func TestModelSelectionDuplicateCompletionIgnored(t *testing.T) {
	m, _ := selectionModel(t, nil)
	n, cmd := m.Update(ControlIntent("models"))
	m = n.(AppModel)
	done := cmd().(operationComplete)
	m = update(m, done)
	done.Session = navSession(t)
	done.Err = errors.New("duplicate error")
	m = update(m, done)
	if m.Header.State.ID != "" || m.Status.Error != "" {
		t.Fatal("completed operation published twice")
	}
}
func TestSlashModelBusyControlSendPreservesCommand(t *testing.T) {
	m, _ := selectionModel(t, nil)
	m.Busy = true
	m.Composer.Input.SetValue("/model")
	n, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = n.(AppModel)
	if cmd != nil || m.Composer.Input.Value() != "/model" || !strings.Contains(m.Status.Error, "running") {
		t.Fatal("busy slash send lost command")
	}
}
func TestModelSelectionCreateFailureKeepsUnconfiguredWorkspace(t *testing.T) {
	m, store := selectionModel(t, nil)
	m = openModels(t, m)
	m.deps.Create.Store = submissionStore{err: errors.New("disk full")}
	m = chooseModel(t, m)
	if m.Header.State.ID != "" || m.Header.State.Workspace != testWorkspace() {
		t.Fatal("failed creation published aggregate or erased workspace")
	}
	rows, _ := store.List(context.Background())
	if len(rows) != 0 {
		t.Fatal("failed creation persisted session")
	}
}

func TestModelSelectionResizeUpdatesKeyboardPage(t *testing.T) {
	m, _ := selectionModel(t, nil)
	m = openModels(t, m)
	models := make([]domain.AvailableModel, 30)
	for i := range models {
		models[i] = domain.AvailableModel{ID: root.ModelID(fmt.Sprintf("provider/%02d", i)), Name: "Model", TextOutput: true, SupportsTools: true}
	}
	m.Models.SetModels(models)
	m = update(m, tea.WindowSizeMsg{Width: 60, Height: 15})
	m = update(m, tea.KeyPressMsg{Code: tea.KeyPgDown})
	selected, ok := m.Models.SelectedModel()
	if !ok || selected.ID != "provider/08" {
		t.Fatalf("resized page selected %s, want provider/08", selected.ID)
	}
}
func TestModelSelectionMouseSelectionAndSavedSessionFromBare(t *testing.T) {
	m, store := selectionModel(t, nil)
	saved := navSession(t)
	if err := store.Save(context.Background(), saved); err != nil {
		t.Fatal(err)
	}
	n, cmd := m.Update(ControlIntent("sessions"))
	m = drain(t, n.(AppModel), cmd)
	n, cmd = m.Update(ControlIntent("open-session"))
	m = drain(t, n.(AppModel), cmd)
	if m.Header.State.Model != "model" {
		t.Fatal("bare screen could not open matching workspace session")
	}
	m = openModels(t, m)
	m.Composer.Input.SetValue("retained")
	n, cmd = click(t, m, "models-model-0")
	m = drain(t, n.(AppModel), cmd)
	if m.Header.State.Model != "provider/chosen" || m.Composer.Input.Value() != "retained" {
		t.Fatal("mouse selection failed or changed editor")
	}
}

func TestModelSelectionUnsolicitedCompletionIgnored(t *testing.T) {
	m, _ := selectionModel(t, nil)
	m = update(m, operationComplete{Session: navSession(t)})
	if m.Header.State.ID != "" {
		t.Fatal("completion without a current operation published a session")
	}
}

func TestModelSelectionBareScreenLongWorkspaceAtMinimumWidth(t *testing.T) {
	m := bareModel()
	defer m.Close()
	m.Header.State.Workspace = domain.Workspace("/home/user/projects/" + strings.Repeat("long-workspace/", 12))
	m = update(m, tea.WindowSizeMsg{Width: 50, Height: 15})
	view := m.View().Content
	if !strings.Contains(view, "Type /model to choose a model") {
		t.Fatalf("bare screen hides selection guidance: %q", view)
	}
	if !strings.Contains(view, "/home/") || lipgloss.Width(view) > 50 || lipgloss.Height(view) > 15 {
		t.Fatalf("workspace missing or screen overflows: %q", view)
	}
}
