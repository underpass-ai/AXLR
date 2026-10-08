package terminal

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// IncidentCard is the person's approval card for an incident postmortem. It
// is not the tool approval card: it shows the exact draft the reviewer judged,
// and its keys record the person's decision in MADE.
type IncidentCard struct {
	Intact    bool
	CanReturn bool
	Reasoning bool
	// Declining is true while the reason asked is a plan's decline (x)
	// rather than a return (d).
	Declining bool
	Reason    textinput.Model
	// shownFence is the present claim the card last opened for on its own, so
	// closing it is respected until the next draft arrives.
	shownFence string
}

func (m AppModel) incidentRun() (domain.CeremonyRun, bool) {
	if m.deps.Session == nil {
		return domain.CeremonyRun{}, false
	}
	run, live := m.deps.Session.Ceremony()
	return run, live && run.AwaitingPerson()
}

// openIncidentCard reads the awaiting draft and shows it.
func (m AppModel) openIncidentCard() AppModel {
	run, awaiting := m.incidentRun()
	if !awaiting {
		m.Status.Error = m.Theme.T("incident.nothing")
		return m
	}
	driver := m.deps.Start.Continue.Ceremonies
	if driver == nil {
		m.Status.Error = m.Theme.T("incident.unavailable")
		return m
	}
	m.Info = NewTranscript()
	w, h := OverlayBodySize(m.Layout.Width, m.Layout.Height-1)
	m.Info.SetWidth(w)
	m.Info.Viewport.SetHeight(max(1, h-2))
	if run.Plan != nil {
		record, err := driver.PlanProposal(m.lifetime.ctx, *m.deps.Session)
		m.Info.SetContent(planCardContent(run, record, err, m.Theme))
		m.Info.Viewport.GotoTop()
		m.IncidentCard = IncidentCard{Intact: err == nil, CanReturn: application.CanReturn(run), shownFence: run.Fence}
		m.overlay = "incident"
		m.Status.Error = ""
		return m
	}
	if run.Repair != nil {
		// The merge card: nothing to read from the workspace, the pull
		// request is the artifact.
		m.Info.SetContent(repairCardContent(run, m.Theme))
		m.Info.Viewport.GotoTop()
		m.IncidentCard = IncidentCard{Intact: true, CanReturn: application.CanReturn(run), shownFence: run.Fence}
		m.overlay = "incident"
		m.Status.Error = ""
		return m
	}
	draft, intact, err := driver.Draft(m.lifetime.ctx, *m.deps.Session)
	m.Info.SetContent(incidentCardContent(run, draft, intact, err, m.Theme))
	m.Info.Viewport.GotoTop()
	m.IncidentCard = IncidentCard{Intact: err == nil && intact, CanReturn: application.CanReturn(run), shownFence: run.Fence}
	m.overlay = "incident"
	m.Status.Error = ""
	return m
}

// autoOpenIncidentCard opens the card once per draft when nothing else is on
// screen: on load and when a turn leaves the draft with the person.
func (m AppModel) autoOpenIncidentCard() AppModel {
	run, awaiting := m.incidentRun()
	if !awaiting || m.Busy || m.overlay != "" || m.Layout.Width == 0 || m.IncidentCard.shownFence == run.Fence {
		return m
	}
	return m.openIncidentCard()
}

func incidentCardContent(run domain.CeremonyRun, draft string, intact bool, err error, theme Theme) string {
	i := run.Incident
	lines := []string{
		theme.Tf("incident.cardInstance", run.Instance),
		theme.Tf("incident.cardDraft", i.DraftPath, shortDigest(i.DraftDigest)),
		theme.Tf("incident.cardReturns", domain.MaxIncidentReturns-min(i.Returns, domain.MaxIncidentReturns)),
	}
	switch {
	case err != nil:
		lines = append(lines, "", theme.Tf("incident.cardUnreadable", err.Error()))
	case !intact:
		lines = append(lines, "", theme.T("incident.cardChanged"))
	}
	if i.Decided == "approve" {
		lines = append(lines, theme.T("incident.cardResume"))
	}
	return strings.Join(lines, "\n") + "\n\n" + draft
}

func repairCardContent(run domain.CeremonyRun, theme Theme) string {
	r := run.Repair
	lines := []string{
		theme.Tf("incident.cardInstance", run.Instance),
		theme.Tf("repair.cardPullRequest", r.PullRequest, r.URL),
		theme.Tf("repair.cardHead", shortDigest(r.HeadSHA), r.Branch),
		"",
		theme.T("repair.cardBody"),
	}
	if r.Decided == "approve" {
		lines = append(lines, theme.T("repair.cardResume"))
	}
	if r.Cause != "" {
		lines = append(lines, "", r.Cause)
	}
	if r.Criteria != "" {
		lines = append(lines, "", r.Criteria)
	}
	if r.Summary != "" {
		lines = append(lines, "", r.Summary)
	}
	return strings.Join(lines, "\n")
}

func shortDigest(digest string) string {
	if len(digest) > 12 {
		return digest[:12]
	}
	return digest
}

// incidentKey handles a key while the card is open.
func (m AppModel) incidentKey(k tea.KeyPressMsg) (AppModel, tea.Cmd) {
	if m.IncidentCard.Reasoning {
		switch k.String() {
		case "esc":
			m.IncidentCard.Reasoning = false
			return m, nil
		case "enter":
			reason := strings.TrimSpace(m.IncidentCard.Reason.Value())
			if reason == "" {
				m.Status.Error = m.Theme.T(m.cardKey("reasonRequired"))
				return m, nil
			}
			if m.IncidentCard.Declining {
				return m.declinePlan(reason)
			}
			return m.decideIncident(false, reason)
		}
		var cmd tea.Cmd
		m.IncidentCard.Reason, cmd = m.IncidentCard.Reason.Update(k)
		return m, cmd
	}
	switch k.String() {
	case "a":
		if !m.IncidentCard.Intact {
			m.Status.Error = m.Theme.T("incident.cardChanged")
			return m, nil
		}
		return m.decideIncident(true, "")
	case "d":
		if !m.IncidentCard.CanReturn {
			m.Status.Error = m.Theme.T("incident.noReturns")
			return m, nil
		}
		return m.askReason("reasonPrompt", false), nil
	case "x":
		if run, awaiting := m.incidentRun(); awaiting && run.Plan != nil {
			return m.askReason("declinePrompt", true), nil
		}
	}
	m.Info.Viewport, _ = m.Info.Viewport.Update(k)
	return m, nil
}

// askReason opens the reason input of a return or a plan's decline.
func (m AppModel) askReason(prompt string, declining bool) AppModel {
	input := textinput.New()
	input.Prompt = m.Theme.T(m.cardKey(prompt))
	input.CharLimit = 2000
	input.SetWidth(max(1, m.Layout.Width-len(input.Prompt)-6))
	input.Focus()
	m.IncidentCard.Reason, m.IncidentCard.Reasoning, m.IncidentCard.Declining = input, true, declining
	return m
}

// declinePlan records the person's x on a plan.
func (m AppModel) declinePlan(reason string) (AppModel, tea.Cmd) {
	if m.Busy {
		m.Status.Error = m.Theme.T("mode.busy")
		return m, nil
	}
	m.overlay = ""
	m.IncidentCard.Reasoning, m.IncidentCard.Declining = false, false
	m.Status.Error = ""
	start := m.deps.Start
	cmd := m.BeginOperation(func(ctx context.Context, s *domain.Session, emit func(application.Event) error) error {
		return start.Decline(ctx, s, reason, emit)
	})
	return m, cmd
}

// decideIncident records the person's decision and starts the turn that
// carries the ceremony on.
func (m AppModel) decideIncident(approve bool, reason string) (AppModel, tea.Cmd) {
	if m.Busy {
		m.Status.Error = m.Theme.T("mode.busy")
		return m, nil
	}
	m.overlay = ""
	m.IncidentCard.Reasoning = false
	m.Status.Error = ""
	start := m.deps.Start
	cmd := m.BeginOperation(func(ctx context.Context, s *domain.Session, emit func(application.Event) error) error {
		return start.Decide(ctx, s, approve, reason, emit)
	})
	return m, cmd
}

func (m AppModel) incidentCardView() (string, string, string) {
	subtitle := m.Theme.T(m.cardKey("hintsApprove"))
	if m.IncidentCard.CanReturn {
		subtitle = m.Theme.T(m.cardKey("hints"))
	}
	body := m.Info.View()
	if m.IncidentCard.Reasoning {
		body += "\n\n" + m.IncidentCard.Reason.View()
		subtitle = m.Theme.T(m.cardKey("reasonHints"))
		if m.IncidentCard.Declining {
			subtitle = m.Theme.T(m.cardKey("declineHints"))
		}
	}
	return m.Theme.T(m.cardKey("title")), subtitle, body
}

// cardKey picks the incident or repair wording for the shared card.
func (m AppModel) cardKey(suffix string) string {
	if run, awaiting := m.incidentRun(); awaiting && run.Repair != nil {
		return "repair." + suffix
	}
	if run, awaiting := m.incidentRun(); awaiting && run.Plan != nil {
		return "plan." + suffix
	}
	return "incident." + suffix
}

// planCardContent shows the verified plan as a table the person decides on.
func planCardContent(run domain.CeremonyRun, record domain.PlanRecord, err error, theme Theme) string {
	if err != nil {
		return theme.Tf("plan.cardUnreadable", err.Error())
	}
	lines := []string{
		theme.Tf("plan.cardHeader", record.ID, len(record.Tasks), record.Waves, record.Planner, record.Worker),
		theme.Tf("plan.cardE2E", strings.TrimSpace(record.E2E.Program+" "+strings.Join(record.E2E.Args, " ")), record.E2EBaseline),
		theme.Tf("plan.cardReturns", domain.MaxPlanReturns-min(run.Plan.Returns, domain.MaxPlanReturns)),
		"",
	}
	for _, t := range record.Tasks {
		first := "—"
		if t.TestFirst {
			first = theme.T("plan.cardTestFirst")
		}
		deps := "—"
		if len(t.DependsOn) > 0 {
			deps = strings.Join(t.DependsOn, ", ")
		}
		lines = append(lines,
			theme.Tf("plan.cardTask", t.Wave, t.ID, t.Goal),
			theme.Tf("plan.cardTaskDetail", strings.Join(t.Scope, ", "), strings.TrimSpace(t.UnitCheck.Program+" "+strings.Join(t.UnitCheck.Args, " ")), t.Baseline, first, deps),
		)
	}
	if strings.TrimSpace(record.Interfaces) != "" {
		lines = append(lines, "", theme.T("plan.cardInterfaces"), record.Interfaces)
	}
	lines = append(lines, "", theme.T("plan.cardBody"))
	return strings.Join(lines, "\n")
}
