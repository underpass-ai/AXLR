package terminal

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	zone "github.com/lrstanley/bubblezone/v2"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func pickerModels() []domain.AvailableModel {
	return []domain.AvailableModel{
		{ID: root.ModelID("openai/alpha"), Name: root.Text("Alpha"), SupportsTools: true, TextOutput: true},
		{ID: root.ModelID("anthropic/beta"), Name: root.Text("Beta"), SupportsTools: true, TextOutput: true},
		{ID: root.ModelID("google/gamma"), Name: root.Text("Gamma"), SupportsTools: true, TextOutput: true},
	}
}

func pickerKey(p ModelPicker, z *zone.Manager, key tea.KeyPressMsg) (ModelPicker, ControlIntent) {
	next, intent, _ := p.Update(key, z, "picker-")
	return next, intent
}

func pickerZone(t *testing.T, z *zone.Manager, id string) *zone.ZoneInfo {
	t.Helper()
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		if hit := z.Get(id); hit != nil {
			return hit
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("missing zone %s", id)
	return nil
}

func TestModelPickerSearchNameIDProvider(t *testing.T) {
	for _, tc := range []struct{ query, wantID string }{
		{"alp", "openai/alpha"}, {"BETA", "anthropic/beta"}, {"google", "google/gamma"},
	} {
		p := NewModelPicker()
		p.SetModels(pickerModels())
		z := zone.New()
		for _, r := range tc.query {
			p, _ = pickerKey(p, z, tea.KeyPressMsg{Code: r, Text: string(r)})
		}
		selected, ok := p.SelectedModel()
		if !ok || string(selected.ID) != tc.wantID {
			t.Fatalf("query %q selected %+v, %v", tc.query, selected, ok)
		}
		view := p.View(z, "picker-", 70, 20)
		if !strings.Contains(view, tc.wantID) || strings.Contains(view, "anthropic/beta") && tc.wantID != "anthropic/beta" {
			t.Fatalf("query %q view: %q", tc.query, view)
		}
	}
}

func TestModelPickerKeyboardMouseSelectionAndClose(t *testing.T) {
	p := NewModelPicker()
	p.SetModels(pickerModels())
	z := zone.New()
	p, _ = pickerKey(p, z, tea.KeyPressMsg{Code: tea.KeyDown})
	p, intent := pickerKey(p, z, tea.KeyPressMsg{Code: tea.KeyEnter})
	if intent != ModelSelectIntent {
		t.Fatalf("enter intent %q", intent)
	}
	selected, _ := p.SelectedModel()
	if selected.ID != "anthropic/beta" {
		t.Fatalf("keyboard selected %s", selected.ID)
	}
	p = NewModelPicker()
	p.SetModels(pickerModels())
	_ = z.Scan(p.View(z, "picker-", 70, 20))
	row := pickerZone(t, z, "picker-model-1")
	p, intent, _ = p.Update(tea.MouseClickMsg{X: row.StartX, Y: row.StartY, Button: tea.MouseLeft}, z, "picker-")
	selected, _ = p.SelectedModel()
	if intent != ModelSelectIntent || selected.ID != "anthropic/beta" {
		t.Fatalf("mouse selected %s, intent %q", selected.ID, intent)
	}
	p, intent = pickerKey(p, z, tea.KeyPressMsg{Code: tea.KeyEsc})
	if intent != ModelCloseIntent {
		t.Fatalf("esc intent %q", intent)
	}
	_ = z.Scan(p.View(z, "picker-", 70, 20))
	close := pickerZone(t, z, "picker-close")
	_, intent, _ = p.Update(tea.MouseClickMsg{X: close.StartX, Y: close.StartY, Button: tea.MouseLeft}, z, "picker-")
	if intent != ModelCloseIntent {
		t.Fatalf("close click intent %q", intent)
	}
}

func TestModelPickerLoadingEmptyErrorRetry(t *testing.T) {
	p := NewModelPicker()
	z := zone.New()
	p.SetLoading(true)
	if !strings.Contains(p.View(z, "picker-", 70, 20), "Loading") {
		t.Fatal("missing loading state")
	}
	p.SetLoading(false)
	p.SetModels(nil)
	if !strings.Contains(p.View(z, "picker-", 70, 20), "No models") {
		t.Fatal("missing empty state")
	}
	p.SetError(errors.New("catalog unavailable"))
	if !strings.Contains(p.View(z, "picker-", 70, 20), "catalog unavailable") {
		t.Fatal("missing error")
	}
	p, intent := pickerKey(p, z, tea.KeyPressMsg{Code: 'r', Text: "r"})
	if intent != ModelRetryIntent {
		t.Fatalf("retry key intent %q", intent)
	}
	_ = z.Scan(p.View(z, "picker-", 70, 20))
	retry := pickerZone(t, z, "picker-retry")
	_, intent, _ = p.Update(tea.MouseClickMsg{X: retry.StartX, Y: retry.StartY, Button: tea.MouseLeft}, z, "picker-")
	if intent != ModelRetryIntent {
		t.Fatalf("retry click intent %q", intent)
	}
	if _, ok := p.SelectedModel(); ok {
		t.Fatal("error state selected a model")
	}
}

func TestModelPickerPageNavigationAndVisibleWindow(t *testing.T) {
	p := NewModelPicker()
	models := make([]domain.AvailableModel, 30)
	for i := range models {
		models[i] = domain.AvailableModel{ID: root.ModelID(fmt.Sprintf("vendor/model-%02d", i)), Name: root.Text(fmt.Sprintf("Model %02d", i))}
	}
	p.SetModels(models)
	z := zone.New()
	_ = p.View(z, "picker-", 40, 10)
	p, _ = pickerKey(p, z, tea.KeyPressMsg{Code: tea.KeyPgDown})
	view := p.View(z, "picker-", 40, 10)
	selected, _ := p.SelectedModel()
	if selected.ID == models[0].ID || !strings.Contains(view, string(selected.ID)) {
		t.Fatalf("page down selection %s not visible: %q", selected.ID, view)
	}
	p, _ = pickerKey(p, z, tea.KeyPressMsg{Code: tea.KeyPgUp})
	selected, _ = p.SelectedModel()
	if selected.ID != models[0].ID {
		t.Fatalf("page up selected %s", selected.ID)
	}
}

func TestModelPickerBoundsSanitizationAndResize(t *testing.T) {
	p := NewModelPicker()
	models := pickerModels()
	models[1].Name = root.Text("界🙂\x1b]2;INJECT\x07\x1b[31mBeta\nOther")
	p.SetModels(models)
	z := zone.New()
	for _, size := range [][2]int{{70, 20}, {40, 10}} {
		view := z.Scan(p.View(z, "picker-", size[0], size[1]))
		if lipgloss.Width(view) > size[0] || lipgloss.Height(view) > size[1] {
			t.Fatalf("overflow at %v: %dx%d", size, lipgloss.Width(view), lipgloss.Height(view))
		}
		if strings.Contains(view, "\x1b]") || strings.Contains(view, "\x1b[31m") || strings.Contains(view, "INJECT") {
			t.Fatalf("terminal sequence leaked: %q", view)
		}
	}
	for _, r := range "beta" {
		p, _ = pickerKey(p, z, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	before, _ := p.SelectedModel()
	p, _ = pickerKey(p, z, tea.KeyPressMsg{Code: tea.KeyDown})
	_ = p.View(z, "picker-", 40, 10)
	after, _ := p.SelectedModel()
	if before.ID != after.ID || !strings.Contains(p.View(z, "picker-", 70, 20), "beta") {
		t.Fatalf("resize lost query/selection: %s to %s", before.ID, after.ID)
	}
}
