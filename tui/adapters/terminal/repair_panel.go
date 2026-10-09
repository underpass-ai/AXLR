package terminal

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// RepairPanelPort is the self-repair coordinator as the console shows it:
// the records linked to the session, the two decisions the person keeps and
// the recovery of an interrupted repair.
type RepairPanelPort interface {
	Records(ctx context.Context) ([]domain.RepairRecord, error)
	Decide(ctx context.Context, id string, approve bool, reason string) error
	Recover(ctx context.Context, id string) error
	Events() <-chan application.RepairEvent
}

// repairEventMsg carries one record change into the program.
type repairEventMsg application.RepairEvent

// RepairPanel shows the self-repairs the agent requested from this session,
// and the ones that wait for the person wherever they were requested. It is
// not the tool approval card: the repair session's calls are approved here.
type RepairPanel struct {
	// Records is the registry as last read: at launch, when the panel opens
	// and on every event. Renders never read the registry.
	Records   []domain.RepairRecord
	Reasoning bool
	Reason    textinput.Model
	// shown is the record and status the panel last opened for on its own,
	// so closing it is respected until the next decision arrives.
	shown string
	// mode is what n starts: the mode of the /repair or /improve that
	// opened the panel, empty when the panel opened on its own.
	mode domain.WorkMode
}

// readRepairEvent waits for the next record change.
func readRepairEvent(ch <-chan application.RepairEvent) tea.Cmd {
	if ch == nil {
		return nil
	}
	return func() tea.Msg {
		event, ok := <-ch
		if !ok {
			return nil
		}
		return repairEventMsg(event)
	}
}

func (m AppModel) subscribeRepairs() tea.Cmd {
	if m.deps.Repairs == nil {
		return nil
	}
	return readRepairEvent(m.deps.Repairs.Events())
}

// loadRepairs reads the registry into the panel's cache.
func (m AppModel) loadRepairs() AppModel {
	if m.deps.Repairs == nil {
		return m
	}
	records, err := m.deps.Repairs.Records(m.lifetime.ctx)
	if err != nil {
		m.Status.Error = err.Error()
		return m
	}
	m.RepairPanel.Records = records
	return m
}

// repairRecords lists, from the cache, the repairs this session asked for
// plus any repair that waits for the person or was interrupted, newest first.
func (m AppModel) repairRecords() []domain.RepairRecord {
	if m.deps.Repairs == nil {
		return nil
	}
	var out []domain.RepairRecord
	for _, record := range m.RepairPanel.Records {
		if record.Parent == m.Header.State.ID || record.Session == m.Header.State.ID || record.Status.Awaiting() || record.Status == domain.RepairInterrupted || record.Status == domain.RepairRunning {
			out = append(out, record)
		}
	}
	return out
}

// actionableRepair is the record the keys act on: the first that waits for a
// decision, else the first interrupted one.
func actionableRepair(records []domain.RepairRecord) (domain.RepairRecord, bool) {
	for _, record := range records {
		if record.Status.Awaiting() {
			return record, true
		}
	}
	for _, record := range records {
		if record.Status == domain.RepairInterrupted {
			return record, true
		}
	}
	return domain.RepairRecord{}, false
}

// openRepairPanel reads the records and shows them.
func (m AppModel) openRepairPanel() AppModel {
	if m.deps.Repairs == nil {
		m.Status.Error = m.Theme.T("repairs.unavailable")
		return m
	}
	m = m.loadRepairs()
	records := m.repairRecords()
	if len(records) == 0 {
		m.Status.Error = m.Theme.T("repairs.nothing")
		return m
	}
	m.RepairPanel.mode = ""
	m.Info = NewTranscript()
	w, h := OverlayBodySize(m.Layout.Width, m.Layout.Height-1)
	m.Info.SetWidth(w)
	m.Info.Viewport.SetHeight(max(1, h-2))
	m.Info.SetContent(repairPanelContent(records, m.Theme))
	m.Info.Viewport.GotoTop()
	if record, ok := actionableRepair(records); ok {
		m.RepairPanel.shown = record.ID + ":" + string(record.Status) + ":" + record.Pending
	}
	m.overlay = "repairs"
	m.Status.Error = ""
	return m
}

// refreshRepairPanel redraws an open panel after a record changed.
func (m AppModel) refreshRepairPanel() AppModel {
	if m.overlay != "repairs" {
		return m
	}
	m = m.loadRepairs()
	records := m.repairRecords()
	if len(records) == 0 {
		m.overlay = ""
		return m
	}
	offset := m.Info.Viewport.YOffset()
	m.Info.SetContent(repairPanelContent(records, m.Theme))
	m.Info.Viewport.SetYOffset(offset)
	return m
}

// autoOpenRepairPanel opens the panel once per decision when nothing else is
// on screen: the reproduction command or the merge waits for the person.
func (m AppModel) autoOpenRepairPanel() AppModel {
	if m.deps.Repairs == nil || m.Busy || m.overlay != "" || m.Layout.Width == 0 || m.inlineApproval() {
		return m
	}
	records := m.repairRecords()
	record, ok := actionableRepair(records)
	if !ok || !record.Status.Awaiting() {
		return m
	}
	key := record.ID + ":" + string(record.Status) + ":" + record.Pending
	if m.RepairPanel.shown == key {
		return m
	}
	return m.openRepairPanel()
}

func repairPanelContent(records []domain.RepairRecord, theme Theme) string {
	var lines []string
	for i, record := range records {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, theme.Accent(theme.Tf(kindKey("repairs.row", record), record.ID, theme.T("repairs.status."+string(record.Status)))))
		lines = append(lines, theme.Tf("repairs.step", orDash(record.Step), orDash(record.State), record.Attempt, record.Repository))
		if record.PullRequest > 0 {
			lines = append(lines, theme.Tf("repairs.pullRequest", record.PullRequest, record.URL))
		}
		if record.MergeSHA != "" {
			lines = append(lines, theme.Tf("repairs.merged", shortDigest(record.MergeSHA)))
		}
		if record.Pending != "" {
			lines = append(lines, theme.Tf("repairs.pending", record.Pending))
		}
		if record.Error != "" {
			lines = append(lines, theme.Tf("repairs.error", record.Error))
		}
		if record.Memory != "" {
			lines = append(lines, theme.Tf("repairs.memory", record.Memory))
		}
		if record.Instance != "" {
			lines = append(lines, theme.Tf("repairs.instance", record.Instance))
		}
		if record.Clone != "" {
			lines = append(lines, theme.Tf("repairs.clone", record.Clone, orDash(string(record.Session))))
		}
		if record.Status == domain.RepairCompleted {
			lines = append(lines, theme.Tf("repairs.restart", orDash(record.Build)))
		}
		if record.Status == domain.RepairInterrupted {
			lines = append(lines, theme.T("repairs.recoverHint"))
		}
	}
	return strings.Join(lines, "\n")
}

func orDash(text string) string {
	if text == "" {
		return "–"
	}
	return text
}

// repairKey handles a key while the panel is open.
func (m AppModel) repairKey(k tea.KeyPressMsg) (AppModel, tea.Cmd) {
	if m.RepairPanel.Reasoning {
		switch k.String() {
		case "esc":
			m.RepairPanel.Reasoning = false
			return m, nil
		case "enter":
			reason := strings.TrimSpace(m.RepairPanel.Reason.Value())
			if reason == "" {
				m.Status.Error = m.Theme.T("repairs.reasonRequired")
				return m, nil
			}
			m.RepairPanel.Reasoning = false
			return m.decideRepair(false, reason), nil
		}
		var cmd tea.Cmd
		m.RepairPanel.Reason, cmd = m.RepairPanel.Reason.Update(k)
		return m, cmd
	}
	record, ok := actionableRepair(m.repairRecords())
	switch k.String() {
	case "a":
		if !ok || !record.Status.Awaiting() {
			m.Status.Error = m.Theme.T("repairs.nothingToDecide")
			return m, nil
		}
		return m.decideRepair(true, ""), nil
	case "d":
		if !ok || !record.Status.Awaiting() {
			m.Status.Error = m.Theme.T("repairs.nothingToDecide")
			return m, nil
		}
		if record.Status == domain.RepairAwaitingApproval {
			return m.decideRepair(false, ""), nil
		}
		input := textinput.New()
		input.Prompt = m.Theme.T("repairs.reasonPrompt")
		input.CharLimit = 2000
		input.SetWidth(max(1, m.Layout.Width-len(input.Prompt)-6))
		input.Focus()
		m.RepairPanel.Reason, m.RepairPanel.Reasoning = input, true
		return m, nil
	case "r":
		if !ok || record.Status != domain.RepairInterrupted {
			m.Status.Error = m.Theme.T("repairs.nothingToRecover")
			return m, nil
		}
		if err := m.deps.Repairs.Recover(m.lifetime.ctx, record.ID); err != nil {
			m.Status.Error = err.Error()
			return m, nil
		}
		m.Status.Error = ""
		return m.refreshRepairPanel(), nil
	case "n":
		// The records are shown before the mode changes; n still starts a
		// new repair or improvement.
		if m.RepairPanel.mode != "" {
			m.overlay = ""
			return m.switchMode(m.RepairPanel.mode)
		}
	}
	m.Info.Viewport, _ = m.Info.Viewport.Update(k)
	return m, nil
}

// decideRepair hands the person's decision to the repair session.
func (m AppModel) decideRepair(approve bool, reason string) AppModel {
	record, ok := actionableRepair(m.repairRecords())
	if !ok {
		m.Status.Error = m.Theme.T("repairs.nothingToDecide")
		return m
	}
	if err := m.deps.Repairs.Decide(m.lifetime.ctx, record.ID, approve, reason); err != nil {
		m.Status.Error = err.Error()
		return m
	}
	m.Status.Error = ""
	return m.refreshRepairPanel()
}

func (m AppModel) repairPanelView() (string, string, string) {
	subtitle := m.Theme.T("repairs.hints")
	switch m.RepairPanel.mode {
	case domain.ModeRepair:
		subtitle = m.Theme.T("repairs.hintsNew")
	case domain.ModeImprove:
		subtitle = m.Theme.T("repairs.hintsNewImprovement")
	}
	body := m.Info.View()
	if m.RepairPanel.Reasoning {
		body += "\n\n" + m.RepairPanel.Reason.View()
		subtitle = m.Theme.T("repairs.reasonHints")
	}
	return m.Theme.T("repairs.title"), subtitle, body
}

// repairBadge is the footer's word about a repair that is not over.
func (m AppModel) repairBadge() string {
	for _, record := range m.repairRecords() {
		if record.Status.Terminal() {
			continue
		}
		return m.Theme.Tf(kindKey("repairs.badge", record), shortRepairID(record.ID), m.Theme.T("repairs.status."+string(record.Status)))
	}
	return ""
}

// kindKey picks the improvement wording of a repair key for an improvement.
func kindKey(key string, record domain.RepairRecord) string {
	if record.Improvement {
		return key + "Improvement"
	}
	return key
}

// improvementRecords reports whether the panel's records include an
// improvement, which /improve then opens instead of changing the mode.
func improvementRecords(records []domain.RepairRecord) bool {
	for _, record := range records {
		if record.Improvement {
			return true
		}
	}
	return false
}

func shortRepairID(id string) string {
	if i := strings.IndexByte(id, '-'); i >= 0 {
		if j := strings.IndexByte(id[i+1:], '-'); j >= 0 {
			rest := id[i+1+j+1:]
			if len(rest) > 18 {
				rest = rest[:18] + "…"
			}
			return rest
		}
	}
	return fmt.Sprint(id)
}
