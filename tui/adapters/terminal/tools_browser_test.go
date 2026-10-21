package terminal

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func browserOverview() application.CatalogOverview {
	catalog := newCatalogFake("business_days", "seat_allocation", "csv_stats")
	catalog.index.Tools[2].Category = "data"
	catalog.index.Tools[2].Summary = "Per-column statistics of a CSV, like csvkit's csvstat: inferred type, nulls, distinct values, top values, min, max, sum, mean and median."
	catalog.index.Tools[2].Requires.Packages = []string{"pandas"}
	return application.CatalogOverview{
		Installed: []application.ForgedTool{
			{Name: "word_count", Description: "Count the words."},
			{Name: "seat_allocation", Origin: &application.ToolOrigin{Catalog: "axlr", Version: "0.9.0"}},
		},
		Updates:  []application.ToolUpdate{{Name: "seat_allocation", Catalog: "axlr", Installed: "0.9.0", Available: "1.0.0"}},
		Catalogs: []application.CatalogState{{Name: "axlr", Index: &catalog.index}},
	}
}

func browserKey(b ToolsBrowser, keys ...tea.KeyPressMsg) (ToolsBrowser, toolsAction) {
	var action toolsAction
	for _, k := range keys {
		b, action, _ = b.Update(k)
	}
	return b, action
}

// Every theme draws the browser with its own palette; a monochrome
// terminal gets no colour codes at all.
func TestToolsBrowserFollowsTheTheme(t *testing.T) {
	for _, id := range []domain.ThemeID{domain.ThemeInk, domain.ThemeAurora, domain.ThemePaper, domain.ThemeEditorial, domain.ThemePhosphor} {
		b := newToolsBrowserOverview(browserOverview(), English)
		theme := Theme{ID: id, Locale: English}
		view := b.View(theme, nil, "", 110, 20)
		accent := sgrForeground(themePalettes[id].Accent)
		if !strings.Contains(view, strings.TrimSuffix(accent, "m")) {
			t.Fatalf("%s: the accent %q is not drawn", id, themePalettes[id].Accent)
		}
		for i, line := range strings.Split(view, "\n") {
			if w := ansi.StringWidth(line); w > 110 {
				t.Fatalf("%s: line %d is %d wide", id, i, w)
			}
		}
	}
	b := newToolsBrowserOverview(browserOverview(), English)
	view := b.View(Theme{Monochrome: true, Locale: English}, nil, "", 110, 20)
	if strings.Contains(view, "\x1b[") {
		t.Fatalf("monochrome has colour codes:\n%q", view)
	}
	if !strings.Contains(view, "[All 4]") || !strings.Contains(view, ">^ seat_allocation") || !strings.Contains(view, "[administration]") {
		t.Fatalf("monochrome marks:\n%s", view)
	}
}

// The sections narrow the list, the filter narrows it further, and the
// details follow the cursor.
func TestToolsBrowserSectionsFilterAndDetails(t *testing.T) {
	b := newToolsBrowserOverview(browserOverview(), English)
	theme := Theme{ID: domain.ThemeInk, Locale: English}
	b.SelectName("word_count")
	view := ansi.Strip(b.View(theme, nil, "", 120, 22))
	for _, want := range []string{"All 4", "Installed 2", "Updates 1", "administration 2", "data 1", "4 of 4", "forged in this workspace"} {
		if !strings.Contains(view, want) {
			t.Fatalf("lacks %q:\n%s", want, view)
		}
	}
	b, _ = browserKey(b, tea.KeyPressMsg{Code: tea.KeyTab}, tea.KeyPressMsg{Code: tea.KeyTab})
	if got := len(b.List.VisibleItems()); got != 1 {
		t.Fatalf("updates lists %d", got)
	}
	view = ansi.Strip(b.View(theme, nil, "", 120, 22))
	if !strings.Contains(view, "update 0.9.0 → 1.0.0 · catalog axlr") || !strings.Contains(view, "⏎ update") {
		t.Fatalf("update details:\n%s", view)
	}
	_, action := browserKey(b, tea.KeyPressMsg{Code: tea.KeyEnter})
	if action.kind != "install" || action.name != "seat_allocation" || action.catalog != "axlr" {
		t.Fatalf("enter = %+v", action)
	}
	// Back to All, then the data tool's details.
	b, _ = browserKey(b, tea.KeyPressMsg{Code: tea.KeyLeft}, tea.KeyPressMsg{Code: tea.KeyLeft})
	b.SelectName("csv_stats")
	view = ansi.Strip(b.View(theme, nil, "", 120, 22))
	for _, want := range []string{"not installed · catalog axlr", "needs  pandas", "Per-column statistics"} {
		if !strings.Contains(view, want) {
			t.Fatalf("details lack %q:\n%s", want, view)
		}
	}
	// A narrow terminal drops the side card for two summary lines.
	if narrow := ansi.Strip(b.View(theme, nil, "", 60, 22)); !strings.Contains(narrow, "Per-column") || strings.Contains(narrow, "│") {
		t.Fatalf("narrow:\n%s", narrow)
	}
	// The filter matches whole terms, the named tools first.
	ranks := filterTools("csv", []string{"word_count count words", "csv_stats data csv statistics", "format_convert data JSON and CSV"})
	if len(ranks) != 2 || ranks[0].Index != 1 || ranks[1].Index != 2 {
		t.Fatalf("filter ranks %+v", ranks)
	}
	// Esc clears an applied filter before it closes.
	b.List.SetFilterText("csv")
	if got := len(b.List.VisibleItems()); got != 1 {
		t.Fatalf("filter lists %d", got)
	}
	b, action = browserKey(b, tea.KeyPressMsg{Code: tea.KeyEscape})
	if action.kind != "" || len(b.List.VisibleItems()) != 4 {
		t.Fatalf("esc with a filter: %+v, %d", action, len(b.List.VisibleItems()))
	}
	if _, action = browserKey(b, tea.KeyPressMsg{Code: tea.KeyEscape}); action.kind != "close" {
		t.Fatalf("esc = %+v", action)
	}
}

// x asks first and removes on the second x; an installed tool at its
// newest version is not installed again.
func TestToolsBrowserRemovesOnConfirmation(t *testing.T) {
	b := newToolsBrowserOverview(browserOverview(), English)
	b.SelectName("word_count")
	b, action := browserKey(b, tea.KeyPressMsg{Code: 'x', Text: "x"})
	if action.kind != "" || b.confirm != "word_count" {
		t.Fatalf("first x = %+v", action)
	}
	if view := ansi.Strip(b.View(Theme{Locale: English}, nil, "", 110, 20)); !strings.Contains(view, "Remove word_count") {
		t.Fatalf("no question:\n%s", view)
	}
	if _, action = browserKey(b, tea.KeyPressMsg{Code: 'x', Text: "x"}); action.kind != "remove" || action.name != "word_count" {
		t.Fatalf("second x = %+v", action)
	}
	b, _ = browserKey(b, tea.KeyPressMsg{Code: tea.KeyDown})
	if b.confirm != "" {
		t.Fatal("another key kept the question")
	}
	b.SelectName("word_count")
	if _, action = browserKey(b, tea.KeyPressMsg{Code: tea.KeyEnter}); action.kind != "" {
		t.Fatalf("enter on a forged tool = %+v", action)
	}
}

// Enter in the console's browser shows the install card, and y brings the
// browser back on the installed tool.
func TestToolsBrowserInstallsAndComesBack(t *testing.T) {
	forge, catalog := &forgeFake{}, newCatalogFake("business_days", "seat_allocation")
	m := toolsModel(t, forge, catalog)
	defer m.zones.Close()
	m = runTools(t, m, "/tools")
	m.Tools.SelectName("seat_allocation")
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = drain(t, next.(AppModel), cmd)
	if !m.toolsAsking || !strings.Contains(m.Info.Text(), "business_days") && !strings.Contains(m.Info.Text(), "seat_allocation") {
		t.Fatalf("no card: asking %v", m.toolsAsking)
	}
	next, cmd = m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m = drain(t, next.(AppModel), cmd)
	if len(forge.tools) != 1 || !m.toolsBrowsing || m.overlay != "tools" {
		t.Fatalf("after y: tools %+v, browsing %v, overlay %q", forge.tools, m.toolsBrowsing, m.overlay)
	}
	if row, ok := m.Tools.Selected(); !ok || row.name != "seat_allocation" || row.state != rowInstalled {
		t.Fatalf("selected %+v", row)
	}
	// x twice removes it, and the browser reads the workspace again.
	m = update(m, tea.KeyPressMsg{Code: 'x', Text: "x"})
	next, cmd = m.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	m = drain(t, next.(AppModel), cmd)
	if len(forge.tools) != 0 || !m.toolsBrowsing {
		t.Fatalf("remove: %+v, browsing %v", forge.tools, m.toolsBrowsing)
	}
	if row, _ := m.Tools.Selected(); row.state != rowAvailable {
		t.Fatalf("after remove %+v", row)
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.overlay != "" {
		t.Fatalf("esc left %q", m.overlay)
	}
}

// The row drawn as selected is the tool Enter installs, past the first
// page of a long catalog too: the stored list is sized, not only the copy
// the view draws.
func TestToolsBrowserHighlightMatchesTheSelectedToolPastAPage(t *testing.T) {
	names := make([]string, 40)
	for i := range names {
		names[i] = fmt.Sprintf("tool_%02d", i)
	}
	m := toolsModel(t, &forgeFake{}, newCatalogFake(names...))
	defer m.zones.Close()
	m = runTools(t, m, "/tools")
	for i := 0; i < 15; i++ {
		m = update(m, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	row, ok := m.Tools.Selected()
	if !ok || row.name != "tool_15" {
		t.Fatalf("selected %+v", row)
	}
	var drawn string
	for _, line := range strings.Split(ansi.Strip(m.View().Content), "\n") {
		if f := strings.Fields(line); len(f) > 1 && (strings.HasPrefix(f[0], "▌") || strings.HasPrefix(f[0], ">")) {
			drawn = f[1]
		}
	}
	if drawn != "tool_15" {
		t.Fatalf("drawn %q, selected %q", drawn, row.name)
	}
}
