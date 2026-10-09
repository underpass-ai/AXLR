package terminal

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/underpass-ai/AXLR/tui/domain"
)

const (
	ModelSelectIntent ControlIntent = "model-select"
	ModelRetryIntent  ControlIntent = "model-retry"
	ModelCloseIntent  ControlIntent = "model-close"
	// ModelFavoriteIntent asks the root to pin or unpin the selected model.
	ModelFavoriteIntent ControlIntent = "model-favorite"
)

// ModelPicker owns local search and display state. Catalog loading and selection
// effects belong to the root model.
type ModelPicker struct {
	Theme     Theme
	Input     textinput.Model
	models    []domain.AvailableModel
	providers []string
	provider  int
	visible   []int
	selected  int
	window    int
	pageSize  int
	loading   bool
	errText   string
	// current is the session's model: marked in the list and selected when
	// the catalogue arrives.
	current string
	// favorites are listed first and marked with a star.
	favorites map[string]bool
}

// SetFavorites lists the given models first, keeping the selected model.
func (p *ModelPicker) SetFavorites(ids []string) {
	selected, ok := p.SelectedModel()
	p.favorites = make(map[string]bool, len(ids))
	for _, id := range ids {
		p.favorites[id] = true
	}
	p.filter()
	if ok {
		for i, index := range p.visible {
			if p.models[index].ID == selected.ID {
				p.selected = i
				break
			}
		}
	}
	p.ensureVisible()
}

func (p *ModelPicker) SetCurrent(id string) { p.current = id }

func NewModelPicker() ModelPicker {
	input := textinput.New()
	input.Prompt = Translate(English, "common.searchPrompt")
	input.SetVirtualCursor(false)
	input.Focus()
	return ModelPicker{Input: input, pageSize: 1}
}

func (p *ModelPicker) SetLoading(loading bool) {
	p.loading = loading
	if loading {
		p.errText = ""
	}
}

func (p *ModelPicker) SetError(err error) {
	p.loading = false
	p.errText = ""
	if err != nil {
		p.errText = singleLine(err.Error())
	}
}

func (p *ModelPicker) SetModels(models []domain.AvailableModel) {
	selected, ok := p.SelectedModel()
	p.models = append([]domain.AvailableModel(nil), models...)
	p.providers = []string{p.Theme.T("filters.all")}
	seen := map[string]bool{}
	for _, model := range p.models {
		provider := strings.SplitN(string(model.ID), "/", 2)[0]
		if !seen[provider] {
			seen[provider] = true
			p.providers = append(p.providers, provider)
		}
	}
	sort.Strings(p.providers[1:])
	p.provider = min(p.provider, len(p.providers)-1)
	p.loading = false
	p.errText = ""
	p.filter()
	want := p.current
	if ok {
		want = string(selected.ID)
	}
	for i, index := range p.visible {
		if string(p.models[index].ID) == want {
			p.selected = i
			break
		}
	}
	p.ensureVisible()
}

func (p ModelPicker) SelectedModel() (domain.AvailableModel, bool) {
	if p.loading || p.errText != "" || p.selected < 0 || p.selected >= len(p.visible) {
		return domain.AvailableModel{}, false
	}
	return p.models[p.visible[p.selected]], true
}

func (p *ModelPicker) filter() {
	// Every word must match, so "anthropic opus" narrows without cycling
	// providers one at a time.
	terms := strings.Fields(strings.ToLower(p.Input.Value()))
	p.visible = p.visible[:0]
	for i, model := range p.models {
		id := singleLine(string(model.ID))
		name := singleLine(string(model.Name))
		provider := strings.SplitN(id, "/", 2)[0]
		if p.provider > 0 && p.provider < len(p.providers) && provider != p.providers[p.provider] {
			continue
		}
		haystack := strings.ToLower(name + " " + id)
		matches := true
		for _, term := range terms {
			if !strings.Contains(haystack, term) {
				matches = false
				break
			}
		}
		if matches {
			p.visible = append(p.visible, i)
		}
	}
	// Favorites first, then the current model, so both open in view.
	rank := func(i int) int {
		id := string(p.models[i].ID)
		switch {
		case p.favorites[id]:
			return 0
		case id == p.current:
			return 1
		}
		return 2
	}
	sort.SliceStable(p.visible, func(a, b int) bool { return rank(p.visible[a]) < rank(p.visible[b]) })
	if len(p.visible) == 0 {
		p.selected = 0
		p.window = 0
	} else if p.selected >= len(p.visible) {
		p.selected = len(p.visible) - 1
	}
	p.ensureVisible()
}

func (p *ModelPicker) ensureVisible() {
	if p.selected < p.window {
		p.window = p.selected
	}
	if p.selected >= p.window+max(1, p.pageSize) {
		p.window = p.selected - max(1, p.pageSize) + 1
	}
	p.window = max(0, min(p.window, max(0, len(p.visible)-max(1, p.pageSize))))
}

func (p ModelPicker) Update(msg tea.Msg, zones *zone.Manager, prefix string) (ModelPicker, ControlIntent, tea.Cmd) {
	if key, ok := msg.(tea.KeyPressMsg); ok {
		// "Retry R" is printed uppercase: Shift+R is the same key. Other
		// letters still reach the search input as typed.
		switch strings.ToLower(key.String()) {
		case "esc":
			return p, ModelCloseIntent, nil
		case "enter":
			if _, ok := p.SelectedModel(); ok {
				return p, ModelSelectIntent, nil
			}
			return p, "", nil
		case "up", "down", "pgup", "pgdown":
			if len(p.visible) > 0 {
				delta := 1
				if key.String() == "up" || key.String() == "pgup" {
					delta = -1
				}
				if key.String() == "pgup" || key.String() == "pgdown" {
					delta *= max(1, p.pageSize)
				}
				p.selected = max(0, min(len(p.visible)-1, p.selected+delta))
				p.ensureVisible()
			}
			return p, "", nil
		case "r":
			if p.errText != "" || (!p.loading && len(p.models) == 0) {
				return p, ModelRetryIntent, nil
			}
		case "ctrl+s":
			if _, ok := p.SelectedModel(); ok {
				return p, ModelFavoriteIntent, nil
			}
			return p, "", nil
		case "tab":
			if len(p.providers) > 1 {
				p.provider = (p.provider + 1) % len(p.providers)
				p.selected, p.window = 0, 0
				p.filter()
			}
			return p, "", nil
		case "shift+tab":
			if len(p.providers) > 1 {
				p.provider = (p.provider + len(p.providers) - 1) % len(p.providers)
				p.selected, p.window = 0, 0
				p.filter()
			}
			return p, "", nil
		}
		old := p.Input.Value()
		var cmd tea.Cmd
		p.Input, cmd = p.Input.Update(key)
		if p.Input.Value() != old {
			p.selected = 0
			p.window = 0
			p.filter()
		}
		return p, "", cmd
	}
	if paste, ok := msg.(tea.PasteMsg); ok {
		old := p.Input.Value()
		var cmd tea.Cmd
		p.Input, cmd = p.Input.Update(paste)
		if p.Input.Value() != old {
			p.selected = 0
			p.window = 0
			p.filter()
		}
		return p, "", cmd
	}
	if click, ok := msg.(tea.MouseClickMsg); ok && click.Button == tea.MouseLeft && zones != nil {
		if zones.Get(prefix + "close").InBounds(click) {
			return p, ModelCloseIntent, nil
		}
		if zones.Get(prefix+"retry").InBounds(click) && (p.errText != "" || (!p.loading && len(p.models) == 0)) {
			return p, ModelRetryIntent, nil
		}
		if !p.loading && p.errText == "" {
			for i := p.window; i < min(len(p.visible), p.window+max(1, p.pageSize)); i++ {
				if zones.Get(fmt.Sprintf("%smodel-%d", prefix, i)).InBounds(click) {
					p.selected = i
					return p, ModelSelectIntent, nil
				}
			}
		}
	}
	return p, "", nil
}

func (p *ModelPicker) View(zones *zone.Manager, prefix string, width, height int) string {
	width, height = max(1, width), max(1, height)
	p.Input.Prompt = p.Theme.T("common.searchPrompt")
	p.Input.SetWidth(max(1, width-12))
	wide := width >= 80
	listWidth := width
	if wide {
		listWidth = max(30, width*55/100)
	}
	p.pageSize = max(1, (height-5)/2)
	p.ensureVisible()
	provider := p.Theme.T("filters.all")
	if p.provider < len(p.providers) {
		provider = p.providers[p.provider]
	}
	lines := []string{p.Theme.Heading(p.Theme.Tf("models.count", len(p.visible))) + "  " + p.Theme.Muted(p.Theme.T("models.hint"))}
	lines = append(lines, p.Input.View())
	lines = append(lines, p.Theme.Muted(p.Theme.Tf("models.providerHint", provider)))
	var left []string
	switch {
	case p.loading:
		left = append(left, p.Theme.Muted(p.Theme.T("models.loading")))
	case p.errText != "":
		left = append(left, p.Theme.T("status.error")+p.errText)
		left = append(left, zones.Mark(prefix+"retry", "["+p.Theme.T("models.retry")+"]"))
	case len(p.visible) == 0:
		left = append(left, p.Theme.T("models.noResults"))
		if len(p.models) == 0 {
			left = append(left, zones.Mark(prefix+"retry", "["+p.Theme.T("models.retry")+"]"))
		}
	default:
		for i := p.window; i < min(len(p.visible), p.window+p.pageSize); i++ {
			model := p.models[p.visible[i]]
			marker := "  "
			if i == p.selected {
				marker = "› "
			}
			name := singleLine(string(model.Name))
			if p.favorites[string(model.ID)] {
				name = p.Theme.Icon("favorite") + " " + name
			}
			if string(model.ID) == p.current {
				name += p.Theme.Accent("  " + p.Theme.T("models.current"))
			}
			label := ansi.Truncate(marker+name, listWidth, "…")
			if i == p.selected {
				label = p.Theme.Selected(label)
			}
			left = append(left, zones.Mark(fmt.Sprintf("%smodel-%d", prefix, i), label))
			meta := "  " + singleLine(string(model.ID))
			if model.Context.Tokens() > 0 {
				meta += p.Theme.Tf("models.contextShort", formatContext(model.Context.Tokens()))
			}
			if in, out := shortRate(model.PromptRate.Display()), shortRate(model.CompletionRate.Display()); in != "" && out != "" {
				meta += " · " + in + "/" + out
			}
			left = append(left, p.Theme.Muted(ansi.Truncate(meta, listWidth, "…")))
		}
	}
	if wide {
		detailWidth := max(1, width-listWidth-2)
		detail := p.detail(detailWidth)
		bodyHeight := max(1, height-5)
		leftBody := lipgloss.NewStyle().Width(listWidth).Height(bodyHeight).Render(strings.Join(left, "\n"))
		rightBody := p.Theme.Panel(lipgloss.NewStyle().Height(bodyHeight).Render(detail), detailWidth)
		lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Top, leftBody, "  ", rightBody))
	} else {
		lines = append(lines, left...)
		if selected, ok := p.SelectedModel(); ok && height >= 14 {
			lines = append(lines, p.Theme.Muted(ansi.Truncate(p.Theme.T("models.detailsPrefix")+string(selected.ID), width, "…")))
		}
	}
	lines = append(lines, zones.Mark(prefix+"close", "["+p.Theme.T("common.close")+"]"))
	lines = strings.Split(strings.Join(lines, "\n"), "\n")
	if len(lines) > height {
		lines = append(lines[:height-1], lines[len(lines)-1])
	}
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, width, "…")
	}
	return strings.Join(lines, "\n")
}

func (p ModelPicker) detail(width int) string {
	model, ok := p.SelectedModel()
	if !ok {
		return p.Theme.Muted(p.Theme.T("models.emptyDetail"))
	}
	lines := []string{p.Theme.Heading(singleLine(string(model.Name))), ""}
	lines = append(lines, strings.Split(ansi.Wrap(singleLine(string(model.ID)), width, ""), "\n")...)
	lines = append(lines, "", p.Theme.Tf("models.context", model.Context.Tokens()))
	if rate := model.PromptRate.Display(); rate != "" {
		lines = append(lines, p.Theme.Tf("models.inputRate", rate))
	}
	if rate := model.CompletionRate.Display(); rate != "" {
		lines = append(lines, p.Theme.Tf("models.outputRate", rate))
	}
	lines = append(lines, p.Theme.T("models.toolsYes"), "", p.Theme.Accent(p.Theme.T("models.setDefault")))
	return strings.Join(lines, "\n")
}

// formatContext shortens a context window: 131k, 1M.
func formatContext(tokens int) string {
	if tokens >= 1_000_000 && tokens%1_000_000 < 100_000 {
		return fmt.Sprintf("%dM", tokens/1_000_000)
	}
	if tokens >= 1000 {
		return fmt.Sprintf("%dk", (tokens+500)/1000)
	}
	return fmt.Sprint(tokens)
}

// shortRate keeps the dollar figure of a per-million rate ("$3 / 1M tokens").
func shortRate(display string) string {
	return strings.TrimSpace(strings.SplitN(display, " /", 2)[0])
}
