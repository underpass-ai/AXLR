package terminal

import (
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func assertComposerColors(t *testing.T, c Composer) {
	t.Helper()
	p := c.Theme.palette()
	screen := composerScreen(c)
	for row := 1; row <= 2; row++ {
		cell := screen.CellAt(2, row)
		if cell == nil || cell.Content == "" || cell.Content == " " {
			t.Fatalf("input text missing from row %d", row)
		}
		assertComposerColor(t, cell.Style.Fg, lipgloss.Color(p.Text))
		assertComposerColor(t, cell.Style.Bg, lipgloss.Color(p.Background))
	}
}

func composerScreen(c Composer) uv.ScreenBuffer {
	screen := uv.NewScreenBuffer(50, c.Input.Height()+1)
	uv.NewStyledString(c.View(50)).Draw(screen, screen.Bounds())
	return screen
}

func assertComposerColor(t *testing.T, got, want color.Color) {
	t.Helper()
	if got == nil {
		t.Fatal("input color is unset")
	}
	gr, gg, gb, ga := got.RGBA()
	wr, wg, wb, wa := want.RGBA()
	if gr != wr || gg != wg || gb != wb || ga != wa {
		t.Fatalf("input color = %v, want %v", got, want)
	}
}

func TestComposerRendersWithThemeColors(t *testing.T) {
	for _, theme := range []Theme{
		{ID: domain.ThemePaper}, {ID: domain.ThemeEditorial},
		{ID: domain.ThemeInk}, {ID: domain.ThemeAurora}, {ID: domain.ThemePhosphor},
		{ID: domain.ThemeAuto}, {ID: domain.ThemeAuto, Light: true},
	} {
		t.Run(string(theme.ID)+map[bool]string{true: "-light"}[theme.Light], func(t *testing.T) {
			c := NewComposer(false)
			c.ApplyTheme(theme)
			c.Input.SetWidth(50)
			c.Input.SetValue("No se lee bien\nsegunda línea")
			assertComposerColors(t, c)
			assertComposerColor(t, c.Input.Cursor().Color, lipgloss.Color(theme.palette().Accent))
			c.Input.Blur()
			assertComposerColors(t, c)
			c.Input.Focus()
			c.Input.Reset()
			screen := composerScreen(c)
			assertComposerColor(t, screen.CellAt(2, 1).Style.Fg, lipgloss.Color(theme.palette().Muted))
			assertComposerColor(t, screen.CellAt(2, 1).Style.Bg, lipgloss.Color(theme.palette().Background))
		})
	}
}

func TestComposerFollowsThemePreviewCancelAndTerminalDetection(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm-256color")
	m := update(New(Dependencies{UIPreferences: domain.UIPreferences{Theme: domain.ThemePaper}}), tea.WindowSizeMsg{Width: 100, Height: 30})
	defer m.zones.Close()
	m.Composer.Input.SetValue("No se lee bien\nsegunda línea")
	assertComposerColors(t, m.Composer)
	m = update(m, ControlIntent("theme"))
	m = update(m, tea.KeyPressMsg{Code: tea.KeyUp})
	if m.Theme.ID != domain.ThemeAurora {
		t.Fatalf("preview theme = %s", m.Theme.ID)
	}
	assertComposerColors(t, m.Composer)
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.Theme.ID != domain.ThemePaper {
		t.Fatalf("cancel theme = %s", m.Theme.ID)
	}
	assertComposerColors(t, m.Composer)
	assertComposerColor(t, m.View().Cursor.Color, lipgloss.Color(m.Theme.palette().Accent))
	m.UIPreferences.Theme = domain.ThemeAuto
	m = update(m, tea.BackgroundColorMsg{Color: lipgloss.Color("#F7F3E9")})
	assertComposerColors(t, m.Composer)
	if m.Theme.palette() != themePalettes[domain.ThemePaper] {
		t.Fatal("automatic theme did not detect the light background")
	}
}

func TestComposerMonochromeRemovesThemeColors(t *testing.T) {
	c := NewComposer(false)
	c.ApplyTheme(Theme{ID: domain.ThemePaper})
	c.ApplyTheme(Theme{Monochrome: true})
	c.Input.SetValue("No se lee bien")
	if view := c.View(50); strings.Contains(view, "\x1b") || !strings.Contains(view, "No se lee bien") {
		t.Fatalf("monochrome input = %q", view)
	}
}
