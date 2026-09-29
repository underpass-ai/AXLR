package terminal

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone/v2"
	"github.com/underpass-ai/AXLR/tui/domain"
)

const (
	ModelSelectIntent ControlIntent = "model-select"
	ModelRetryIntent  ControlIntent = "model-retry"
	ModelCloseIntent  ControlIntent = "model-close"
)

// ModelPicker owns local search and display state. Catalog loading and selection
// effects belong to the root model.
type ModelPicker struct {
	Input    textinput.Model
	models   []domain.AvailableModel
	visible  []int
	selected int
	window   int
	pageSize int
	loading  bool
	errText  string
}

func NewModelPicker() ModelPicker {
	input := textinput.New()
	input.Prompt = "Search: "
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
	p.loading = false
	p.errText = ""
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

func (p ModelPicker) SelectedModel() (domain.AvailableModel, bool) {
	if p.loading || p.errText != "" || p.selected < 0 || p.selected >= len(p.visible) {
		return domain.AvailableModel{}, false
	}
	return p.models[p.visible[p.selected]], true
}

func (p *ModelPicker) filter() {
	query := strings.ToLower(strings.TrimSpace(p.Input.Value()))
	p.visible = p.visible[:0]
	for i, model := range p.models {
		id := singleLine(string(model.ID))
		name := singleLine(string(model.Name))
		provider := strings.SplitN(id, "/", 2)[0]
		if query == "" || strings.Contains(strings.ToLower(name), query) || strings.Contains(strings.ToLower(id), query) || strings.Contains(strings.ToLower(provider), query) {
			p.visible = append(p.visible, i)
		}
	}
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
		switch key.String() {
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
	p.Input.SetWidth(max(1, width-9))
	p.pageSize = max(1, height-5)
	p.ensureVisible()
	lines := []string{"Models — ↑↓ / PgUp PgDn / Enter"}
	lines = append(lines, ansi.Truncate(p.Input.View(), width, "…"))
	switch {
	case p.loading:
		lines = append(lines, "Loading models…")
	case p.errText != "":
		lines = append(lines, ansi.Truncate("Error: "+p.errText, width, "…"))
		lines = append(lines, zones.Mark(prefix+"retry", "[Retry R]"))
	case len(p.visible) == 0:
		lines = append(lines, "No models found")
		if len(p.models) == 0 {
			lines = append(lines, zones.Mark(prefix+"retry", "[Retry R]"))
		}
	default:
		for i := p.window; i < min(len(p.visible), p.window+p.pageSize); i++ {
			model := p.models[p.visible[i]]
			marker := "  "
			if i == p.selected {
				marker = "> "
			}
			label := fmt.Sprintf("%s%s | %s", marker, singleLine(string(model.Name)), singleLine(string(model.ID)))
			if model.Context.Tokens() > 0 {
				label += fmt.Sprintf(" | %d ctx", model.Context.Tokens())
			}
			if rate := model.PromptRate.Display(); rate != "" {
				label += " | in " + rate
			}
			if rate := model.CompletionRate.Display(); rate != "" {
				label += " | out " + rate
			}
			lines = append(lines, zones.Mark(fmt.Sprintf("%smodel-%d", prefix, i), ansi.Truncate(label, width, "…")))
		}
	}
	lines = append(lines, zones.Mark(prefix+"close", "[Close Esc]"))
	if len(lines) > height {
		lines = append(lines[:height-1], lines[len(lines)-1])
	}
	return strings.Join(lines, "\n")
}
