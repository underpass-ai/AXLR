package terminal

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type uiPreferenceStub struct {
	saved domain.UIPreferences
	err   error
}

func (s *uiPreferenceStub) Load(context.Context) (domain.UIPreferences, error) { return s.saved, nil }
func (s *uiPreferenceStub) Save(_ context.Context, p domain.UIPreferences) error {
	if s.err != nil {
		return s.err
	}
	s.saved = p
	return nil
}

func TestThemePickerPreviewCancelAndSave(t *testing.T) {
	store := &uiPreferenceStub{}
	m := sized()
	defer m.zones.Close()
	m.deps.UIPreferenceStore = store
	m = update(m, ControlIntent("theme"))
	m = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	if m.Theme.ID != domain.ThemeInk || m.UIPreferences.Theme != domain.ThemeAuto {
		t.Fatal("preview changed committed preference")
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.overlay != "" || m.Theme.ID != domain.ThemeAuto {
		t.Fatal("cancel did not restore theme")
	}
	m = update(m, ControlIntent("theme"))
	m = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	m = update(m, tea.KeyPressMsg{Code: 'i', Text: "i"})
	m = update(m, tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.overlay != "" || store.saved.Theme != domain.ThemeInk || store.saved.Icons != domain.IconsNerd || !store.saved.ReduceMotion {
		t.Fatalf("theme not saved: %+v", store.saved)
	}
}

func TestThemePickerSaveFailureKeepsOverlay(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m.deps.UIPreferenceStore = &uiPreferenceStub{err: errors.New("disk full")}
	m = update(m, ControlIntent("theme"))
	m = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.overlay != "theme" || m.UIPreferences.Theme != domain.ThemeAuto || !strings.Contains(m.Status.Error, "disk full") {
		t.Fatal("failed save appeared committed")
	}
}
