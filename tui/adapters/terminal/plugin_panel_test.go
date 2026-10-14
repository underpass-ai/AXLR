package terminal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type pluginPanelStub struct {
	items    []domain.PluginState
	err      error
	toggles  int
	manifest string
	urlID    root.PluginID
	url      string
}

func (p *pluginPanelStub) List(context.Context) ([]domain.PluginState, error) { return p.items, p.err }
func (p *pluginPanelStub) InstallManifest(_ context.Context, path string) error {
	p.manifest = path
	return nil
}
func (p *pluginPanelStub) InstallURL(_ context.Context, id root.PluginID, url string) error {
	p.urlID = id
	p.url = url
	return nil
}
func (p *pluginPanelStub) SetApproval(_ context.Context, id root.PluginID, mode domain.ApprovalMode) error {
	p.toggles++
	if p.err != nil {
		return p.err
	}
	for i := range p.items {
		if p.items[i].Profile.ID == id {
			p.items[i].Profile.Approval = mode
		}
	}
	return nil
}
func pluginItem(id root.PluginID) domain.PluginState {
	return domain.PluginState{Profile: domain.PluginProfile{ID: id, Name: root.Text(id), Purpose: domain.PluginPurposeMemory, Approval: domain.ApprovalManual}, Tools: []root.PluginToolName{"kmp_ask", "kmp_wake"}}
}
func runUIOperation(m AppModel, cmd tea.Cmd) AppModel {
	for cmd != nil {
		var next tea.Model
		next, cmd = m.Update(cmd())
		m = next.(AppModel)
	}
	return m
}
func TestMCPCommandAndExplicitPolicyToggle(t *testing.T) {
	for _, command := range []string{"/mcp"} {
		t.Run(command, func(t *testing.T) {
			manager := &pluginPanelStub{items: []domain.PluginState{pluginItem("kmp")}}
			m := sized()
			defer m.zones.Close()
			m.deps.Plugins = manager
			before := len(m.Header.State.Messages)
			m.Composer.Input.SetValue(command)
			next, cmd := m.Update(ControlIntent("send"))
			m = next.(AppModel)
			if cmd == nil || !m.Busy || m.Composer.Input.Value() != "" {
				t.Fatal("management command was not dispatched")
			}
			m = runUIOperation(m, cmd)
			if len(m.Header.State.Messages) != before || !strings.Contains(m.View().Content, "kmp_wake") {
				t.Fatal("command became chat history or inventory missing")
			}
			next, cmd = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
			m = next.(AppModel)
			if cmd != nil || manager.toggles != 0 || !strings.Contains(m.View().Content, "Enter confirm") {
				t.Fatal("policy changed before confirmation")
			}
			next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			m = next.(AppModel)
			m = runUIOperation(m, cmd)
			if manager.toggles != 1 || m.Plugins.Items[0].Profile.Approval != domain.ApprovalAuto {
				t.Fatal("policy did not become automatic")
			}
			next, cmd = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
			m = next.(AppModel)
			next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			m = runUIOperation(next.(AppModel), cmd)
			if m.Plugins.Items[0].Profile.Approval != domain.ApprovalManual {
				t.Fatal("policy did not return to manual")
			}
		})
	}
}

func TestMCPInstallAcceptsManifestAndURL(t *testing.T) {
	for _, input := range []string{"/absolute/third-party.json", "search https://example.com/mcp"} {
		t.Run(input, func(t *testing.T) {
			manager := &pluginPanelStub{}
			m := sized()
			defer m.zones.Close()
			m.deps.Plugins = manager
			next, cmd := m.Update(ControlIntent("mcp"))
			m = runUIOperation(next.(AppModel), cmd)
			next, _ = m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
			m = next.(AppModel)
			m.Plugins.InstallInput.SetValue(input)
			next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			m = runUIOperation(next.(AppModel), cmd)
			if input[0] == '/' && manager.manifest != input {
				t.Fatal("manifest not installed")
			}
			if input[0] != '/' && (manager.urlID != "search" || manager.url != "https://example.com/mcp") {
				t.Fatal("URL not installed")
			}
		})
	}
}
func TestPluginPolicyFailureKeepsVisibleManualPolicy(t *testing.T) {
	manager := &pluginPanelStub{items: []domain.PluginState{pluginItem("kmp")}}
	m := sized()
	defer m.zones.Close()
	m.deps.Plugins = manager
	next, cmd := m.Update(ControlIntent("mcp"))
	m = runUIOperation(next.(AppModel), cmd)
	manager.err = errors.New("cannot persist policy")
	next, cmd = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = next.(AppModel)
	next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = runUIOperation(next.(AppModel), cmd)
	if m.Plugins.Items[0].Profile.Approval != domain.ApprovalManual || !strings.Contains(m.View().Content, "cannot persist policy") {
		t.Fatal("failed save appeared successful")
	}
}
func TestPluginInventoryIsScrollableAndBounded(t *testing.T) {
	p := NewPluginPanel()
	var items []domain.PluginState
	for i := range 32 {
		item := pluginItem(root.PluginID(fmt.Sprintf("plugin-%d", i)))
		for j := range 200 {
			item.Tools = append(item.Tools, root.PluginToolName(fmt.Sprintf("tool-%d", j)))
		}
		items = append(items, item)
	}
	p.SetItems(items)
	p.Resize(50, 13)
	for range 31 {
		p.Update(tea.KeyPressMsg{Code: tea.KeyDown}, "plugins")
	}
	view := p.View("plugins", 50, 13)
	if lipgloss.Height(view) > 13 || lipgloss.Width(view) > 50 || !strings.Contains(view, "plugin-31") {
		t.Fatalf("inventory overflow or selection lost: %dx%d", lipgloss.Width(view), lipgloss.Height(view))
	}
	before := p.Details.YOffset()
	p.Update(tea.KeyPressMsg{Code: tea.KeyPgDown}, "plugins")
	if p.Details.YOffset() <= before {
		t.Fatal("tool inventory does not scroll")
	}
}

// A policy save and the following discovery are separate outcomes.
type pluginRefreshFailureStub struct {
	profile domain.PluginProfile
	lists   int
	err     error
}

func (p *pluginRefreshFailureStub) List(context.Context) ([]domain.PluginState, error) {
	p.lists++
	if p.lists > 1 {
		return nil, p.err
	}
	return []domain.PluginState{{Profile: p.profile}}, nil
}
func (p *pluginRefreshFailureStub) SetApproval(_ context.Context, _ root.PluginID, mode domain.ApprovalMode) error {
	p.profile.Approval = mode
	return nil
}
func (p *pluginRefreshFailureStub) InstallManifest(context.Context, string) error { return nil }
func (p *pluginRefreshFailureStub) InstallURL(context.Context, root.PluginID, string) error {
	return nil
}
func TestPluginPolicyRemainsVisibleWhenDiscoveryAfterSaveFails(t *testing.T) {
	for _, err := range []error{errors.New("discovery failed"), context.Canceled} {
		t.Run(err.Error(), func(t *testing.T) {
			manager := &pluginRefreshFailureStub{profile: pluginItem("kmp").Profile, err: err}
			m := sized()
			defer m.zones.Close()
			m.deps.Plugins = manager
			next, cmd := m.Update(ControlIntent("mcp"))
			m = runUIOperation(next.(AppModel), cmd)
			next, cmd = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
			m = next.(AppModel)
			if m.Plugins.Items[0].Profile.Approval != domain.ApprovalManual {
				t.Fatal("policy changed before persistence")
			}
			next, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			m = next.(AppModel)
			m = runUIOperation(m, cmd)
			if manager.profile.Approval != domain.ApprovalAuto || m.Plugins.Items[0].Profile.Approval != domain.ApprovalAuto {
				t.Fatal("successfully persisted policy was hidden by discovery failure")
			}
			if !strings.Contains(m.Plugins.Error, "Approval saved; inventory refresh failed") || !strings.Contains(m.Status.Error, "Approval saved") {
				t.Fatal("failure does not distinguish save from refresh")
			}
		})
	}
}

func TestSpanishInstallInputShowsTheSpanishPlaceholder(t *testing.T) {
	session := navSession(t)
	m := update(New(Dependencies{Session: &session, Monochrome: true, Locale: Spanish}), tea.WindowSizeMsg{Width: 100, Height: 30})
	defer m.zones.Close()
	next, cmd := m.Update(ControlIntent("mcp"))
	m = runUIOperation(next.(AppModel), cmd)
	m = update(m, tea.KeyPressMsg{Code: 'i', Text: "i"})
	if view := m.View().Content; !m.Plugins.installing || !strings.Contains(view, Translate(Spanish, "mcp.manifestPlaceholder")) {
		t.Fatalf("the install input is not in Spanish:\n%s", view)
	}
}

// /mcp's engine facts read in the person's language: the selection reason
// and the identity note were English on 14 October 2026.
func TestEngineFactsAreTranslated(t *testing.T) {
	p := NewPluginPanel()
	p.Theme = Theme{Monochrome: true, Locale: Spanish}
	p.Facts = map[string][]EngineFact{"kmp": {{Key: "mcp.reason", Value: "project", ValueKey: "mcp.reason.project"}, {Key: "mcp.identity", Value: "axlr:1", NoteKey: "mcp.identityNote"}}}
	p.Resize(160, 40)
	p.SetItems([]domain.PluginState{{Profile: domain.PluginProfile{ID: "kmp", Name: "KMP"}}})
	text := p.Details.View()
	for _, want := range []string{"Elegido por: el .kernel del repositorio", "axlr:1 (un agente por almacén"} {
		if !strings.Contains(text, want) {
			t.Fatalf("details lack %q:\n%s", want, text)
		}
	}
}

// On 14 October 2026 /mcp cut the store path ("…4c2") and the work
// identity ("(un agente por almacén, un cont") at the detail's edge.
func TestMCPDetailWrapsLongValues(t *testing.T) {
	store := "/tmp/claude-1000/-home-gx10a-Documents-ai-AXLR/88338e7f-86bd-4af8-a524-88f27f6c4a23/ws/.kernel"
	p := NewPluginPanel()
	p.Theme = Theme{Monochrome: true}
	p.Facts = map[string][]EngineFact{"kmp": {{Key: "mcp.store", Value: store}, {Key: "mcp.identity", Value: "axlr:98fb", NoteKey: "mcp.identityNote"}}}
	p.SetItems([]domain.PluginState{{Profile: domain.PluginProfile{ID: "kmp", Name: "kmp"}}})
	p.Resize(100, 30)
	var joined strings.Builder
	for _, line := range strings.Split(ansi.Strip(p.Details.View()), "\n") {
		joined.WriteString(strings.TrimRight(line, " "))
	}
	note := strings.Fields(Translate(English, "mcp.identityNote"))
	if !strings.Contains(joined.String(), store) || !strings.Contains(joined.String(), note[len(note)-1]) {
		t.Fatalf("a long value was cut:\n%s", ansi.Strip(p.Details.View()))
	}
}
