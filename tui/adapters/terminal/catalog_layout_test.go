package terminal

import (
	"testing"

	"charm.land/lipgloss/v2"
	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestCatalogAndThemeOverlaysFitWideAndNarrowTerminals(t *testing.T) {
	for _, size := range [][2]int{{50, 15}, {70, 20}, {100, 32}} {
		w, h := size[0], size[1]-2
		z := zone.New()
		defer z.Close()
		models := NewModelPicker()
		models.Theme = Theme{ID: domain.ThemeInk}
		models.SetModels(pickerModels())
		plugins := NewPluginPanel()
		plugins.Theme = models.Theme
		plugins.SetItems([]domain.PluginState{pluginItem("kmp")})
		plugins.Resize(w, h)
		picker := NewThemePicker(domain.DefaultUIPreferences())
		for name, view := range map[string]string{
			"models":  models.View(z, "layout-", w, h),
			"plugins": plugins.View("plugins", w, h),
			"theme":   picker.View(models.Theme, w, h),
		} {
			if lipgloss.Width(view) > w || lipgloss.Height(view) > h {
				t.Errorf("%s overflow at %v: %dx%d", name, size, lipgloss.Width(view), lipgloss.Height(view))
			}
		}
	}
}
