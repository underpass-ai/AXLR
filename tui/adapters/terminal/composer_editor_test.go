package terminal

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// On 12 October 2026 Shift+Enter was the only newline, and terminals
// without the kitty keyboard or modifyOtherKeys send it as Enter, which
// sent half a message. Ctrl+J and Alt+Enter work everywhere.
func TestCtrlJAndAltEnterInsertANewline(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{{Code: 'j', Mod: tea.ModCtrl}, {Code: tea.KeyEnter, Mod: tea.ModAlt}, {Code: tea.KeyEnter, Mod: tea.ModShift}} {
		m := sized()
		m = typeDraft(m, "one")
		m = update(m, key)
		m = typeDraft(m, "two")
		if got := m.Composer.Input.Value(); got != "one\ntwo" || m.Busy {
			t.Fatalf("%s: draft %q busy %v", key, got, m.Busy)
		}
		m.zones.Close()
	}
}

// On 12 October 2026 Ctrl+D opened Changes in the middle of a prompt; with
// a draft it now deletes forward, as in the composer's other emacs keys.
func TestCtrlDDeletesForwardInADraftAndOpensChangesWithout(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m = typeDraft(m, "abc")
	m = update(m, tea.KeyPressMsg{Code: tea.KeyHome})
	m = update(m, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if m.overlay != "" || m.Composer.Input.Value() != "bc" {
		t.Fatalf("Ctrl+D with a draft: overlay=%q draft=%q", m.overlay, m.Composer.Input.Value())
	}
	m.Composer.Input.Reset()
	m = update(m, tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if m.overlay != "changes" {
		t.Fatalf("Ctrl+D without a draft opened %q", m.overlay)
	}
}

func TestCtrlGEditsTheDraftInTheEditor(t *testing.T) {
	env := func(values map[string]string) func(string) string {
		return func(name string) string { return values[name] }
	}
	path := filepath.Join(t.TempDir(), "draft.md")
	for _, c := range []struct {
		env  map[string]string
		want []string
	}{
		{map[string]string{"VISUAL": "code --wait", "EDITOR": "nano"}, []string{"code", "--wait", path}},
		{map[string]string{"EDITOR": "nano"}, []string{"nano", path}},
	} {
		if got := editorCommand(env(c.env), path).Args; strings.Join(got, " ") != strings.Join(c.want, " ") {
			t.Fatalf("editor for %v = %v; want %v", c.env, got, c.want)
		}
	}
	if got := editorCommand(env(nil), path).Args; len(got) != 2 || got[1] != path {
		t.Fatalf("fallback editor = %v", got)
	}

	m := sized()
	defer m.zones.Close()
	m = typeDraft(m, "short")
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("Ctrl+G did not open the editor")
	}
	m = update(m, editorDone{Text: "a longer\nprompt\n"})
	if m.Composer.Input.Value() != "a longer\nprompt" {
		t.Fatalf("edited draft = %q", m.Composer.Input.Value())
	}
	m = update(m, editorDone{Err: errors.New("exit status 1")})
	if m.Composer.Input.Value() != "a longer\nprompt" || !strings.Contains(m.Status.Error, "exit status 1") {
		t.Fatalf("a failed editor changed the draft: %q / %q", m.Composer.Input.Value(), m.Status.Error)
	}
}
