package terminal

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// PlanPanelPort is the plan runner as the console shows it.
type PlanPanelPort interface {
	Records(ctx context.Context) ([]domain.PlanRecord, error)
	Start(ctx context.Context, planID string) error
	Events() <-chan application.PlanEvent
}

// planEventMsg carries one plan change into the program.
type planEventMsg application.PlanEvent

// PlanPanel shows the plans: their tasks, waves and syncs. Renders read the
// cache only; it is refreshed on open and on every event.
type PlanPanel struct {
	Records []domain.PlanRecord
}

func readPlanEvent(ch <-chan application.PlanEvent) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		event, ok := <-ch
		if !ok {
			return nil
		}
		return planEventMsg(event)
	}
}

func (m AppModel) subscribePlans() tea.Cmd {
	if m.deps.Plans == nil {
		return nil
	}
	return readPlanEvent(m.deps.Plans.Events())
}

func (m AppModel) loadPlans() AppModel {
	if m.deps.Plans == nil {
		return m
	}
	records, err := m.deps.Plans.Records(m.lifetime.ctx)
	if err != nil {
		m.Status.Error = err.Error()
		return m
	}
	m.PlanPanel.Records = records
	return m
}

// activePlans are the plans that are not over, newest first.
func (m AppModel) activePlans() []domain.PlanRecord {
	var out []domain.PlanRecord
	for _, record := range m.PlanPanel.Records {
		if !record.Status.Terminal() {
			out = append(out, record)
		}
	}
	return out
}

func (m AppModel) openPlanPanel() AppModel {
	m = m.loadPlans()
	if len(m.PlanPanel.Records) == 0 {
		m.Status.Error = m.Theme.T("plans.nothing")
		return m
	}
	m.Info = NewTranscript()
	w, h := OverlayBodySize(m.Layout.Width, m.Layout.Height-1)
	m.Info.SetWidth(w)
	m.Info.Viewport.SetHeight(max(1, h-2))
	m.Info.SetContent(planPanelContent(m.PlanPanel.Records, m.Theme))
	m.Info.Viewport.GotoTop()
	m.overlay = "plans"
	m.Status.Error = ""
	return m
}

func (m AppModel) refreshPlanPanel() AppModel {
	if m.overlay != "plans" {
		return m
	}
	m = m.loadPlans()
	offset := m.Info.Viewport.YOffset()
	m.Info.SetContent(planPanelContent(m.PlanPanel.Records, m.Theme))
	m.Info.Viewport.SetYOffset(offset)
	return m
}

func planPanelContent(records []domain.PlanRecord, theme Theme) string {
	var lines []string
	for i, plan := range records {
		if i >= 8 {
			break
		}
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, theme.Tf("plans.row", plan.ID, theme.T("plans.status."+string(plan.Status)), len(plan.Tasks), plan.Waves))
		if plan.Error != "" {
			lines = append(lines, "  "+plan.Error)
		}
		for _, t := range plan.Tasks {
			detail := theme.T("plans.task." + string(t.Status))
			if t.Status == domain.TaskRunning && t.Step != "" {
				detail += " · " + t.Step
			}
			if t.Reason != "" {
				detail += " · " + t.Reason
			}
			lines = append(lines, theme.Tf("plans.taskRow", t.Wave, t.ID, detail))
			if t.Handback != nil {
				for _, note := range t.Handback.Notes {
					lines = append(lines, theme.Tf("plans.note", note.From, note.To, note.Text))
				}
				for _, q := range t.Handback.Questions {
					lines = append(lines, theme.Tf("plans.question", q))
				}
			}
		}
		for _, s := range plan.Syncs {
			lines = append(lines, theme.Tf("plans.syncRow", s.Wave, s.Verdict, s.Rounds))
		}
	}
	return strings.Join(lines, "\n")
}

// planKey handles the panel's keys: r runs again an interrupted plan, n
// starts a new one.
func (m AppModel) planKey(k tea.KeyPressMsg) (AppModel, tea.Cmd) {
	switch k.String() {
	case "r":
		for _, record := range m.PlanPanel.Records {
			if record.Status == domain.PlanInterrupted {
				if err := m.deps.Plans.Start(m.lifetime.ctx, record.ID); err != nil {
					m.Status.Error = err.Error()
					return m, nil
				}
				m.Status.Error = ""
				return m.refreshPlanPanel(), nil
			}
		}
		m.Status.Error = m.Theme.T("plans.nothingInterrupted")
		return m, nil
	case "n":
		m.overlay = ""
		return m.switchMode(domain.ModePlan)
	}
	m.Info.Viewport, _ = m.Info.Viewport.Update(k)
	return m, nil
}

// planBadge is the footer's word about a running plan:
// plan <slug> · wave 2/3 · t4 green 2/3.
func (m AppModel) planBadge() string {
	for _, plan := range m.activePlans() {
		if plan.Status != domain.PlanRunning {
			continue
		}
		wave, current := 0, ""
		for _, t := range plan.Tasks {
			if t.Status == domain.TaskRunning {
				wave, current = t.Wave, fmt.Sprintf("%s %s", t.ID, t.Step)
			}
		}
		if current == "" {
			return m.Theme.Tf("plans.badgeSync", plan.ID)
		}
		return m.Theme.Tf("plans.badge", plan.ID, wave, plan.Waves, current)
	}
	return ""
}
