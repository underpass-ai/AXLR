package terminal

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// The /tools browser: a list of the workspace's tools and the catalogs',
// one row each, sections across the top (all, installed, updates and each
// category), a filter, and the selected tool's details beside the list.
// Every colour comes from the theme's palette; a monochrome terminal gets
// plain glyphs and the selection marker.

// catalogRowState is where a tool in the browser stands for this workspace.
type catalogRowState int

const (
	rowAvailable catalogRowState = iota
	rowInstalled
	rowUpdate
	rowForged
	rowBroken
)

// catalogRow is one tool in the browser.
type catalogRow struct {
	name, version, category, summary, catalog string
	state                                     catalogRowState
	// installed is the installed version, available the catalog's newer
	// one for an update; broken is why the files no longer match.
	installed, available, broken string
	program, license             string
	tags, packages               []string
	network                      bool
}

func (r catalogRow) FilterValue() string {
	return r.name + " " + r.category + " " + strings.Join(r.tags, " ") + " " + r.summary
}

// installable reports a row Enter installs: a catalog tool not installed,
// or one its catalog has newer.
func (r catalogRow) installable() bool {
	return r.catalog != "" && (r.state == rowAvailable || r.state == rowUpdate)
}

// removable reports a row x removes: a tool registered in the workspace.
func (r catalogRow) removable() bool {
	return r.state != rowAvailable
}

// toolSection is a tab of the browser.
type toolSection struct {
	id, label string
	keep      func(catalogRow) bool
}

// toolsAction is what a key in the browser asks the console to do.
type toolsAction struct {
	kind          string // "install", "remove", "refresh", "close"
	name, catalog string
}

// ToolsBrowser is the /tools panel.
type ToolsBrowser struct {
	List     list.Model
	rows     []catalogRow
	sections []toolSection
	section  int
	warnings []string
	// search is the terms of /tools search; "" for the overview.
	search string
	// confirm names the tool x was pressed on once: x again removes it.
	confirm string
	// notice is a line the footer shows until the next key.
	notice  string
	loading bool
}

// newToolsBrowserOverview lists /tools: the workspace's tools and the
// catalogs'.
func newToolsBrowserOverview(overview application.CatalogOverview, locale Locale) ToolsBrowser {
	updates := map[string]application.ToolUpdate{}
	for _, u := range overview.Updates {
		updates[u.Name] = u
	}
	installed := map[string]application.ForgedTool{}
	for _, tool := range overview.Installed {
		installed[tool.Name] = tool
	}
	var rows []catalogRow
	listed := map[string]bool{}
	var warnings []string
	for _, state := range overview.Catalogs {
		if state.Warning != "" {
			warnings = append(warnings, state.Name+": "+singleLine(state.Warning))
		}
		if state.Index == nil {
			continue
		}
		for _, entry := range application.NewestOfEach(state.Index.Tools) {
			if listed[entry.Name] {
				continue
			}
			row := rowOfEntry(state.Name, entry)
			if tool, ok := installed[entry.Name]; ok && tool.Origin != nil && tool.Origin.Catalog == state.Name {
				listed[entry.Name] = true
				row.state, row.installed = rowInstalled, tool.Origin.Version
				if u, ok := updates[entry.Name]; ok {
					row.state, row.installed, row.available = rowUpdate, u.Installed, u.Available
				}
				if tool.Broken != "" {
					row.state, row.broken = rowBroken, tool.Broken
				}
			} else if _, taken := installed[entry.Name]; taken {
				// A tool forged here, or from another catalog, has the
				// name: the workspace's own is the one listed.
				continue
			}
			rows = append(rows, row)
		}
	}
	for _, tool := range overview.Installed {
		if listed[tool.Name] {
			continue
		}
		row := catalogRow{name: tool.Name, summary: singleLine(tool.Description), program: tool.Program, state: rowForged, category: "forged"}
		if tool.Origin != nil {
			row.state, row.catalog, row.version, row.installed, row.category = rowInstalled, tool.Origin.Catalog, tool.Origin.Version, tool.Origin.Version, "catalog"
		}
		if tool.Broken != "" {
			row.state, row.broken = rowBroken, tool.Broken
		}
		rows = append(rows, row)
	}
	sortRows(rows)
	return newToolsBrowser(rows, warnings, "", locale)
}

// newToolsBrowserSearch lists /tools search's matches.
func newToolsBrowserSearch(terms []string, matches []application.CatalogMatch, catalogs []application.CatalogState, locale Locale) ToolsBrowser {
	rows := make([]catalogRow, 0, len(matches))
	for _, match := range matches {
		row := rowOfEntry(match.Catalog, match.Tool)
		if match.InstalledVersion != "" {
			row.state, row.installed = rowInstalled, match.InstalledVersion
			if match.InstalledVersion != match.Tool.Version {
				row.state, row.available = rowUpdate, match.Tool.Version
			}
		}
		rows = append(rows, row)
	}
	var warnings []string
	for _, state := range catalogs {
		if state.Warning != "" {
			warnings = append(warnings, state.Name+": "+singleLine(state.Warning))
		}
	}
	return newToolsBrowser(rows, warnings, strings.Join(terms, " "), locale)
}

func rowOfEntry(catalog string, entry application.CatalogEntry) catalogRow {
	return catalogRow{
		name: entry.Name, version: entry.Version, category: entry.Category, summary: singleLine(entry.Summary),
		catalog: catalog, program: entry.Program, license: entry.License, tags: entry.Tags,
		packages: entry.Requires.Packages, network: entry.Network,
	}
}

// sortRows puts the workspace's tools first, then the rest by name.
func sortRows(rows []catalogRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i].state != rowAvailable, rows[j].state != rowAvailable
		if a != b {
			return a
		}
		return rows[i].name < rows[j].name
	})
}

func newToolsBrowser(rows []catalogRow, warnings []string, search string, locale Locale) ToolsBrowser {
	l := list.New(nil, toolsDelegate{}, 60, 12)
	l.SetShowTitle(false)
	l.SetShowFilter(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetShowHelp(false)
	l.DisableQuitKeybindings()
	l.FilterInput.Prompt = Translate(locale, "common.searchPrompt")
	l.FilterInput.SetVirtualCursor(false)
	l.Filter = filterTools
	b := ToolsBrowser{List: l, rows: rows, warnings: warnings, search: search}
	b.sections = toolSections(rows, search, locale)
	b.applySection()
	return b
}

// toolSections are the tabs: all (or the search), installed, updates and
// each category, the largest first; empty ones are left out.
func toolSections(rows []catalogRow, search string, locale Locale) []toolSection {
	all := Translate(locale, "tools.tabAll")
	if search != "" {
		all = Translatef(locale, "tools.tabSearch", search)
	}
	sections := []toolSection{{id: "all", label: all, keep: func(catalogRow) bool { return true }}}
	count := func(keep func(catalogRow) bool) int {
		n := 0
		for _, row := range rows {
			if keep(row) {
				n++
			}
		}
		return n
	}
	mine := func(r catalogRow) bool { return r.state != rowAvailable }
	if count(mine) > 0 {
		sections = append(sections, toolSection{id: "installed", label: Translate(locale, "tools.tabInstalled"), keep: mine})
	}
	updates := func(r catalogRow) bool { return r.state == rowUpdate }
	if count(updates) > 0 {
		sections = append(sections, toolSection{id: "updates", label: Translate(locale, "tools.tabUpdates"), keep: updates})
	}
	perCategory := map[string]int{}
	for _, row := range rows {
		if row.state == rowAvailable || row.catalog != "" {
			perCategory[row.category]++
		}
	}
	categories := make([]string, 0, len(perCategory))
	for category := range perCategory {
		if category != "" {
			categories = append(categories, category)
		}
	}
	sort.Slice(categories, func(i, j int) bool {
		if perCategory[categories[i]] != perCategory[categories[j]] {
			return perCategory[categories[i]] > perCategory[categories[j]]
		}
		return categories[i] < categories[j]
	})
	for _, category := range categories {
		category := category
		sections = append(sections, toolSection{id: "category:" + category, label: category, keep: func(r catalogRow) bool { return r.category == category }})
	}
	for i := range sections {
		sections[i].label = fmt.Sprintf("%s %d", sections[i].label, count(sections[i].keep))
	}
	return sections
}

// filterTools keeps the tools whose name, category, tags or summary hold
// every term, those named by them first. The list's fuzzy match let
// "csv" keep 139 of 159 tools, any with c, s and v in order.
func filterTools(term string, targets []string) []list.Rank {
	terms := strings.Fields(strings.ToLower(term))
	var named, rest []list.Rank
	for i, target := range targets {
		lowered := strings.ToLower(target)
		name, _, _ := strings.Cut(lowered, " ")
		all, inName := true, true
		for _, t := range terms {
			if !strings.Contains(lowered, t) {
				all = false
				break
			}
			inName = inName && strings.Contains(name, t)
		}
		if !all {
			continue
		}
		rank := list.Rank{Index: i}
		if inName {
			named = append(named, rank)
		} else {
			rest = append(rest, rank)
		}
	}
	return append(named, rest...)
}

// applySection lists the current section's rows, keeping the selected
// tool when it is still there.
func (b *ToolsBrowser) applySection() tea.Cmd {
	selected := ""
	if row, ok := b.Selected(); ok {
		selected = row.name
	}
	keep := b.sections[b.section].keep
	var items []list.Item
	at := 0
	for _, row := range b.rows {
		if keep(row) {
			if row.name == selected {
				at = len(items)
			}
			items = append(items, row)
		}
	}
	cmd := b.List.SetItems(items)
	b.List.Select(at)
	return cmd
}

// Selected is the tool under the cursor.
func (b ToolsBrowser) Selected() (catalogRow, bool) {
	row, ok := b.List.SelectedItem().(catalogRow)
	return row, ok
}

// SelectName moves the cursor to the tool named, if it is listed.
func (b *ToolsBrowser) SelectName(name string) {
	for i, item := range b.List.VisibleItems() {
		if item.(catalogRow).name == name {
			b.List.Select(i)
			return
		}
	}
}

// Update takes a key or the list's filter messages.
func (b ToolsBrowser) Update(msg tea.Msg) (ToolsBrowser, toolsAction, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		var cmd tea.Cmd
		b.List, cmd = b.List.Update(msg)
		return b, toolsAction{}, cmd
	}
	b.notice = ""
	confirm := b.confirm
	b.confirm = ""
	if b.List.SettingFilter() {
		var cmd tea.Cmd
		b.List, cmd = b.List.Update(msg)
		return b, toolsAction{}, cmd
	}
	row, selected := b.Selected()
	switch strings.ToLower(key.String()) {
	case "esc":
		if b.List.FilterState() == list.FilterApplied {
			b.List.ResetFilter()
			return b, toolsAction{}, nil
		}
		return b, toolsAction{kind: "close"}, nil
	case "tab", "right", "l":
		b.section = (b.section + 1) % len(b.sections)
		return b, toolsAction{}, b.applySection()
	case "shift+tab", "left", "h":
		b.section = (b.section + len(b.sections) - 1) % len(b.sections)
		return b, toolsAction{}, b.applySection()
	case "enter", "i", "u":
		if !selected {
			return b, toolsAction{}, nil
		}
		if !row.installable() {
			b.notice = "current"
			return b, toolsAction{}, nil
		}
		return b, toolsAction{kind: "install", name: row.name, catalog: row.catalog}, nil
	case "x", "delete":
		if !selected || !row.removable() {
			return b, toolsAction{}, nil
		}
		if confirm != row.name {
			b.confirm = row.name
			return b, toolsAction{}, nil
		}
		return b, toolsAction{kind: "remove", name: row.name}, nil
	case "r":
		return b, toolsAction{kind: "refresh"}, nil
	}
	var cmd tea.Cmd
	b.List, cmd = b.List.Update(msg)
	return b, toolsAction{}, cmd
}

// browserPaint styles text on the panel's surface, or on the selection,
// so a reset inside a row never leaves the rest of it unpainted.
type browserPaint struct {
	theme Theme
	bg    string
}

func (p browserPaint) fg(colour string, bold bool, s string) string {
	if p.theme.Monochrome {
		return s
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(colour)).Background(lipgloss.Color(p.bg)).Bold(bold).Render(s)
}

func (p browserPaint) chip(colour, s string) string {
	if p.theme.Monochrome {
		return "[" + s + "]"
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(colour)).Background(lipgloss.Color(p.theme.palette().Raised)).Render(" " + s + " ")
}

// categoryColour gives each category a steady colour among the palette's.
func categoryColour(theme Theme, category string) string {
	p := theme.palette()
	// Red is left out: it reads as an error.
	tones := []string{p.Accent, p.Good, p.Warning}
	sum := 0
	for _, r := range category {
		sum = sum*31 + int(r)
	}
	if sum < 0 {
		sum = -sum
	}
	return tones[sum%len(tones)]
}

// stateGlyph is a row's leading mark and its colour.
func stateGlyph(theme Theme, state catalogRowState) (string, string) {
	p := theme.palette()
	plain := theme.Monochrome || theme.Icons == domain.IconsASCII
	switch state {
	case rowInstalled:
		return pick(plain, "*", "●"), p.Good
	case rowUpdate:
		return pick(plain, "^", "↑"), p.Warning
	case rowForged:
		return pick(plain, "+", "◆"), p.Accent
	case rowBroken:
		return pick(plain, "!", "✖"), p.DiffRemoved
	}
	return pick(plain, "-", "○"), p.Muted
}

func pick(plain bool, ascii, glyph string) string {
	if plain {
		return ascii
	}
	return glyph
}

// toolsDelegate draws one row: mark, name, version, category, summary.
type toolsDelegate struct {
	theme           Theme
	zones           *zone.Manager
	prefix          string
	nameWidth, catW int
	versionW        int
}

func (toolsDelegate) Height() int                         { return 1 }
func (toolsDelegate) Spacing() int                        { return 0 }
func (toolsDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

func (d toolsDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	row, ok := item.(catalogRow)
	if !ok {
		return
	}
	width := max(1, m.Width())
	p := d.theme.palette()
	selected := index == m.Index() && m.FilterState() != list.Filtering
	paint := browserPaint{theme: d.theme, bg: p.Surface}
	if selected {
		paint.bg = p.Selected
	}
	bar := " "
	if selected {
		bar = pick(d.theme.Monochrome || d.theme.Icons == domain.IconsASCII, ">", "▌")
	}
	glyph, colour := stateGlyph(d.theme, row.state)
	nameColour, nameBold := p.Text, row.state != rowAvailable
	if selected {
		nameColour, nameBold = p.Text, true
	}
	version := row.version
	if row.state == rowUpdate {
		version = row.installed + "→" + row.available
	}
	cells := []string{
		paint.fg(p.Accent, true, bar),
		paint.fg(colour, false, glyph+" "),
		paint.fg(nameColour, nameBold, padRight(ansi.Truncate(row.name, d.nameWidth, "…"), d.nameWidth)),
		paint.fg(p.Muted, false, " "+padRight(ansi.Truncate(version, d.versionW, "…"), d.versionW)),
	}
	used := 1 + 2 + d.nameWidth + 1 + d.versionW
	if d.catW > 0 && width-used > d.catW+12 {
		category := padRight(ansi.Truncate(row.category, d.catW, "…"), d.catW)
		cells = append(cells, paint.fg(categoryColour(d.theme, row.category), false, " "+category+" "))
		used += d.catW + 2
	}
	if rest := width - used; rest > 4 {
		summary := padRight(ansi.Truncate(row.summary, rest-1, "…"), rest-1)
		cells = append(cells, paint.fg(p.Muted, false, " "+summary))
		used += rest
	}
	line := strings.Join(cells, "")
	if pad := width - used; pad > 0 {
		line += paint.fg(p.Text, false, strings.Repeat(" ", pad))
	}
	if d.zones != nil {
		line = d.zones.Mark(fmt.Sprintf("%stool-%d", d.prefix, index), line)
	}
	fmt.Fprint(w, line)
}

func padRight(s string, width int) string {
	if gap := width - ansi.StringWidth(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}

// Click selects the row under a left click; it reports whether one was hit.
func (b *ToolsBrowser) Click(zones *zone.Manager, prefix string, click tea.MouseClickMsg) bool {
	if zones == nil || click.Button != tea.MouseLeft {
		return false
	}
	start := b.List.Paginator.Page * b.List.Paginator.PerPage
	end := min(len(b.List.VisibleItems()), start+b.List.Paginator.PerPage)
	for i := start; i < end; i++ {
		if zones.Get(fmt.Sprintf("%stool-%d", prefix, i)).InBounds(click) {
			b.List.Select(i)
			b.confirm = ""
			return true
		}
	}
	return false
}

// Wheel moves the cursor a row per notch.
func (b *ToolsBrowser) Wheel(wheel tea.MouseWheelMsg) {
	switch wheel.Button {
	case tea.MouseWheelUp:
		b.List.CursorUp()
	case tea.MouseWheelDown:
		b.List.CursorDown()
	}
}

// View draws the browser's body inside the overlay: tabs, the list beside
// the details, and the key hints.
func (b *ToolsBrowser) View(theme Theme, zones *zone.Manager, prefix string, width, height int) string {
	p := theme.palette()
	paint := browserPaint{theme: theme, bg: p.Surface}
	width, height = max(20, width), max(6, height)
	lines := []string{b.tabsView(theme, width)}
	if len(b.warnings) > 0 {
		warn := pick(theme.Monochrome || theme.Icons == domain.IconsASCII, "!", "▲")
		text := warn + " " + b.warnings[0]
		if len(b.warnings) > 1 {
			text += fmt.Sprintf(" (+%d)", len(b.warnings)-1)
		}
		lines = append(lines, paint.fg(p.Warning, false, ansi.Truncate(text, width, "…")))
	} else {
		lines = append(lines, paint.fg(p.Border, false, strings.Repeat(pick(theme.Monochrome, "-", "─"), width)))
	}
	listHeight := height - 4
	detailWidth := 0
	if width >= 96 {
		detailWidth = min(64, max(34, width*2/5))
	}
	listWidth := width
	if detailWidth > 0 {
		listWidth = width - detailWidth - 3
	} else {
		listHeight -= 2
	}
	nameWidth, catWidth, versionWidth := 8, 0, 5
	for _, item := range b.List.Items() {
		row := item.(catalogRow)
		nameWidth = max(nameWidth, ansi.StringWidth(row.name))
		catWidth = max(catWidth, ansi.StringWidth(row.category))
		version := row.version
		if row.state == rowUpdate {
			version = row.installed + "→" + row.available
		}
		versionWidth = max(versionWidth, ansi.StringWidth(version))
	}
	nameWidth = min(nameWidth, max(10, listWidth/3))
	catWidth = min(catWidth, 16)
	versionWidth = min(versionWidth, 13)
	b.List.SetDelegate(toolsDelegate{theme: theme, zones: zones, prefix: prefix, nameWidth: nameWidth, catW: catWidth, versionW: versionWidth})
	b.List.SetSize(listWidth, max(1, listHeight))
	listView := b.List.View()
	if len(b.List.VisibleItems()) == 0 {
		empty := theme.T("tools.noneInSection")
		if b.List.FilterState() != list.Unfiltered {
			empty = theme.T("tools.noMatch")
		}
		listView = "  " + paint.fg(p.Muted, false, empty)
	}
	listLines := strings.Split(listView, "\n")
	for len(listLines) < listHeight {
		listLines = append(listLines, "")
	}
	listLines = listLines[:max(1, listHeight)]
	if detailWidth > 0 {
		detail := b.detailLines(theme, detailWidth, listHeight)
		sep := paint.fg(p.Border, false, " "+pick(theme.Monochrome, "|", "│")+" ")
		for i := range listLines {
			left := listLines[i]
			if gap := listWidth - ansi.StringWidth(left); gap > 0 {
				left += paint.fg(p.Text, false, strings.Repeat(" ", gap))
			}
			lines = append(lines, ansi.Truncate(left, listWidth, "")+sep+detail[i])
		}
	} else {
		lines = append(lines, listLines...)
		summary := ""
		if row, ok := b.Selected(); ok {
			summary = row.summary
		}
		wrapped := strings.Split(ansi.Wordwrap(summary, width, " "), "\n")
		for i := 0; i < 2; i++ {
			text := ""
			if i < len(wrapped) {
				text = wrapped[i]
			}
			lines = append(lines, paint.fg(p.Muted, false, ansi.Truncate(text, width, "…")))
		}
	}
	lines = append(lines, "", b.footerView(theme, width))
	return strings.Join(lines, "\n")
}

// tabsView is the section switcher, and the filter when one is typed.
func (b ToolsBrowser) tabsView(theme Theme, width int) string {
	p := theme.palette()
	paint := browserPaint{theme: theme, bg: p.Surface}
	filter := ""
	if b.List.SettingFilter() || b.List.FilterState() == list.FilterApplied {
		filter = "  " + b.List.FilterInput.View()
	} else {
		filter = "  " + paint.fg(p.Muted, false, theme.T("tools.filterHint"))
	}
	room := max(10, width-ansi.StringWidth(filter))
	tabs := make([]string, len(b.sections))
	for i, section := range b.sections {
		if i == b.section {
			if theme.Monochrome {
				tabs[i] = "[" + section.label + "]"
			} else {
				tabs[i] = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(p.Background)).Background(lipgloss.Color(p.Accent)).Render(" " + section.label + " ")
			}
			continue
		}
		tabs[i] = paint.fg(p.Muted, false, " "+section.label+" ")
	}
	// Drop tabs from the left, then the right, until the active one fits.
	first, last := 0, len(tabs)
	joined := func() string { return strings.Join(tabs[first:last], paint.fg(p.Border, false, " ")) }
	for ansi.StringWidth(joined()) > room && last-first > 1 {
		if b.section-first > last-1-b.section {
			first++
		} else {
			last--
		}
	}
	line := joined()
	if first > 0 {
		line = paint.fg(p.Muted, false, "‹ ") + line
	}
	if last < len(tabs) {
		line += paint.fg(p.Muted, false, " ›")
	}
	return ansi.Truncate(line, room, "…") + filter
}

// detailLines is the selected tool's card, height lines of width cells.
func (b ToolsBrowser) detailLines(theme Theme, width, height int) []string {
	p := theme.palette()
	paint := browserPaint{theme: theme, bg: p.Surface}
	var out []string
	add := func(s string) { out = append(out, s) }
	row, ok := b.Selected()
	if ok {
		add(paint.fg(p.Text, true, row.name) + paint.fg(p.Muted, false, "  "+row.version))
		glyph, colour := stateGlyph(theme, row.state)
		var state string
		switch row.state {
		case rowInstalled:
			state = theme.Tf("tools.stateInstalled", row.installed, row.catalog)
		case rowUpdate:
			state = theme.Tf("tools.stateUpdate", row.installed, row.available, row.catalog)
		case rowForged:
			state = theme.T("tools.stateForged")
		case rowBroken:
			state = theme.Tf("tools.stateBroken", row.broken)
		default:
			state = theme.Tf("tools.stateAvailable", row.catalog)
		}
		add(paint.fg(colour, false, glyph+" "+state))
		add("")
		var chips []string
		if row.category != "" {
			chips = append(chips, paint.chip(categoryColour(theme, row.category), row.category))
		}
		if row.program != "" {
			chips = append(chips, paint.chip(p.Text, row.program))
		}
		if row.license != "" {
			chips = append(chips, paint.chip(p.Muted, row.license))
		}
		if row.network {
			chips = append(chips, paint.chip(p.Warning, theme.T("tools.network")))
		}
		if len(chips) > 0 {
			add(strings.Join(chips, paint.fg(p.Text, false, " ")))
			add("")
		}
		for _, line := range strings.Split(ansi.Wordwrap(row.summary, width, " "), "\n") {
			add(paint.fg(p.Text, false, line))
		}
		if len(row.tags) > 0 {
			add("")
			add(paint.fg(p.Muted, false, theme.T("tools.tags")+"  ") + paint.fg(p.Text, false, strings.Join(row.tags, " · ")))
		}
		if len(row.packages) > 0 {
			add(paint.fg(p.Muted, false, theme.T("tools.needs")+"  ") + paint.fg(p.Text, false, strings.Join(row.packages, ", ")))
		}
	}
	var actions []string
	if ok && row.installable() {
		label := theme.T("tools.actionInstall")
		if row.state == rowUpdate {
			label = theme.T("tools.actionUpdate")
		}
		actions = append(actions, paint.chip(p.Good, "⏎ "+label))
	}
	if ok && row.removable() {
		actions = append(actions, paint.chip(p.DiffRemoved, "x "+theme.T("tools.actionRemove")))
	}
	if len(out) > height-2 {
		out = out[:max(0, height-2)]
	}
	for len(out) < height-1 {
		out = append(out, "")
	}
	out = append(out[:height-1], strings.Join(actions, paint.fg(p.Text, false, " ")))
	for i, line := range out {
		line = ansi.Truncate(line, width, "…")
		if gap := width - ansi.StringWidth(line); gap > 0 {
			line += paint.fg(p.Text, false, strings.Repeat(" ", gap))
		}
		out[i] = line
	}
	return out
}

// footerView counts what is listed and names the keys, or asks to confirm
// a removal.
func (b ToolsBrowser) footerView(theme Theme, width int) string {
	p := theme.palette()
	paint := browserPaint{theme: theme, bg: p.Surface}
	if b.confirm != "" {
		return paint.fg(p.DiffRemoved, true, ansi.Truncate(theme.Tf("tools.confirmRemove", b.confirm), width, "…"))
	}
	count := theme.Tf("tools.count", len(b.List.VisibleItems()), len(b.rows))
	if b.loading {
		count = theme.T("tools.refreshing")
	}
	hint := theme.T("tools.keys")
	if b.notice == "current" {
		hint = theme.T("tools.alreadyCurrent")
	}
	return ansi.Truncate(paint.fg(p.Accent, true, count)+paint.fg(p.Muted, false, "  "+hint), width, "…")
}
