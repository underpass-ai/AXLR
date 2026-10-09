package terminal

import (
	"errors"
	"path/filepath"
	"testing"

	tea "charm.land/bubbletea/v2"
	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// shifted is a letter typed with Shift, as the hints print it: Key.String()
// is then the uppercase text.
func shifted(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r - 'a' + 'A'), Mod: tea.ModShift}
}

// The hints print these keys in uppercase; Shift+letter must do what the
// lowercase letter does, as in the /mcp panel.
func TestUppercaseKeysFromTheHintsWork(t *testing.T) {
	t.Run("catalog M adds a source", func(t *testing.T) {
		p := NewInstalledPlugins()
		p.Update(shifted('m'))
		if !p.addingMarketplace {
			t.Fatal("M did not open the source input")
		}
	})
	t.Run("catalog R refreshes", func(t *testing.T) {
		p := NewInstalledPlugins()
		if got := p.Update(shifted('r')); got != "catalog-refresh" {
			t.Fatalf("R: intent %q; want catalog-refresh", got)
		}
	})
	t.Run("theme I cycles icons", func(t *testing.T) {
		p := NewThemePicker(domain.DefaultUIPreferences(), English)
		before := p.Preview.Icons
		p, intent, _ := p.Update(shifted('i'))
		if intent != "theme-preview" || p.Preview.Icons == before {
			t.Fatalf("I: intent %q, icons %q", intent, p.Preview.Icons)
		}
	})
	t.Run("theme A toggles motion", func(t *testing.T) {
		p := NewThemePicker(domain.DefaultUIPreferences(), English)
		p, intent, _ := p.Update(shifted('a'))
		if intent != "theme-preview" || !p.Preview.ReduceMotion {
			t.Fatalf("A: intent %q, reduce motion %v", intent, p.Preview.ReduceMotion)
		}
	})
	t.Run("palette shortcut drawn uppercase", func(t *testing.T) {
		if _, intent, _ := NewActionPalette(English).Update(shifted('m')); intent != "models" {
			t.Fatalf("M: intent %q; want models", intent)
		}
	})
	t.Run("model picker Retry R", func(t *testing.T) {
		z := zone.New()
		defer z.Close()
		p := NewModelPicker()
		p.SetError(errors.New("catalog offline"))
		if _, intent := pickerKey(p, z, shifted('r')); intent != ModelRetryIntent {
			t.Fatalf("R: intent %q; want retry", intent)
		}
	})
	t.Run("approval A L D", func(t *testing.T) {
		var d ApprovalDialog
		for r, want := range map[rune]domain.ToolDecision{'a': domain.DecisionApprove, 'l': domain.DecisionAutoApprove, 'd': domain.DecisionDeny} {
			if got := d.Intent(shifted(r)); got != want {
				t.Fatalf("%c: decision %q; want %q", r-'a'+'A', got, want)
			}
		}
	})
	t.Run("approval F full autonomy", func(t *testing.T) {
		m := approvalModel(t)
		settings, err := storage.NewApprovalSettings(filepath.Join(t.TempDir(), "approvals.json"), nil)
		if err != nil {
			t.Fatal(err)
		}
		m.deps.ApprovalSettings = settings
		m.deps.Resolve.Approval = settings
		m.deps.Resolve.Tools = navTool{}
		m.deps.Resolve.Continue.Models = navStream{}
		next, cmd := m.Update(shifted('f'))
		m = drain(t, next.(AppModel), cmd)
		if !settings.Autonomous() || !m.Status.Autonomous {
			t.Fatal("F did not turn full autonomy on")
		}
	})
}
