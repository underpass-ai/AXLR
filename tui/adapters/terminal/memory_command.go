package terminal

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// /memory (decisions A4, D7, E15 and I3 of the lifecycle integration
// design): save exports the project about as the checkout's bundle,
// compare reads KMP's proposals between the session's about and others,
// forget closes an entry with a supersedes relation. There is no write
// command and no /memory view: the person records through the work, and
// reads with the model or KMP's own viewer.

// memoryCommand runs /memory with what followed it.
func (m AppModel) memoryCommand(args string) (tea.Model, tea.Cmd) {
	fields := strings.Fields(args)
	if len(fields) == 0 {
		// The bare command only names its verbs: the composer keeps it,
		// ready for one, instead of "/memory" plus whatever is typed next.
		m.Composer.Input.SetValue("/memory ")
		m.Composer.Input.CursorEnd()
		m.Status.Notice = m.Theme.Tf("notice.slashUsage", slashUsage("/memory"))
		return m, nil
	}
	if m.Busy || m.knownPending() {
		m.Status.Error = m.Theme.T("mode.busy")
		return m, nil
	}
	switch strings.ToLower(fields[0]) {
	case "save":
		return m.saveMemory()
	case "compare":
		return m.compareMemory(fields[1:])
	case "forget":
		if len(fields) < 2 {
			m.Status.Notice = m.Theme.Tf("notice.slashUsage", slashUsage("/memory"))
			return m, nil
		}
		return m.forgetMemory(fields[1], strings.Join(fields[2:], " "), "/memory "+strings.Join(fields, " "))
	}
	m.Status.Notice = m.Theme.Tf("notice.slashUsage", slashUsage("/memory"))
	return m, nil
}

// sessionAbout is the session's project about: the selected one, else the
// launch's.
func (m AppModel) sessionAbout() string {
	if m.Header.About != "" {
		return m.Header.About
	}
	return m.deps.About
}

// sessionContext is the session's memory context, "" before the first
// prompt.
func (m AppModel) sessionContext() domain.MemoryContextID {
	if instance := m.Header.State.Instance; instance != nil {
		return instance.MemoryContext
	}
	return ""
}

// saveMemory exports the project about as the bundle.
func (m AppModel) saveMemory() (tea.Model, tea.Cmd) {
	if m.deps.MemoryBundle == nil {
		m.Status.Error = m.Theme.T("memory.unavailable")
		return m, nil
	}
	about := m.sessionAbout()
	if about == "" {
		m.Status.Error = m.Theme.T("memory.noAbout")
		return m, nil
	}
	m.Composer.Input.Reset()
	m.Status.Error = ""
	bundles := m.deps.MemoryBundle
	var saved application.BundleSaved
	cmd := m.BeginOperation(func(ctx context.Context, _ *domain.Session, _ func(application.Event) error) error {
		var err error
		saved, err = bundles.Save(ctx, about)
		return err
	})
	return m, func() tea.Msg {
		msg := cmd()
		if done, ok := msg.(operationComplete); ok && done.Err == nil {
			done.MemorySaved = &saved
			return done
		}
		return msg
	}
}

// compareMemory opens the memory panel and reads the comparison into it.
func (m AppModel) compareMemory(others []string) (tea.Model, tea.Cmd) {
	if m.deps.MemoryCompare == nil {
		m.Status.Error = m.Theme.T("memory.unavailable")
		return m, nil
	}
	about := m.sessionAbout()
	if about == "" {
		m.Status.Error = m.Theme.T("memory.noAbout")
		return m, nil
	}
	if len(others) == 0 {
		m.Status.Notice = m.Theme.Tf("notice.slashUsage", slashUsage("/memory"))
		return m, nil
	}
	m.Composer.Input.Reset()
	m.Status.Error = ""
	m.Info = m.themedTranscript()
	w, h := OverlayBodySize(m.Layout.Width, m.Layout.Height-1)
	m.Info.SetWidth(w)
	m.Info.Viewport.SetHeight(h)
	m.Info.SetContent(m.Theme.Tf("memory.comparing", strings.Join(append([]string{about}, others...), " · ")))
	m.Info.Viewport.GotoTop()
	m.overlay = "memory"
	compare := m.deps.MemoryCompare
	request := application.CompareRequest{About: about, Others: others, ContextID: m.sessionContext()}
	var result application.CompareResult
	cmd := m.BeginOperation(func(ctx context.Context, _ *domain.Session, _ func(application.Event) error) error {
		var err error
		result, err = compare.Compare(ctx, request)
		return err
	})
	return m, func() tea.Msg {
		msg := cmd()
		if done, ok := msg.(operationComplete); ok {
			done.MemoryCompared = &result
			return done
		}
		return msg
	}
}

// forgetMemory closes one entry.
func (m AppModel) forgetMemory(ref, why, command string) (tea.Model, tea.Cmd) {
	if m.deps.MemoryForget == nil {
		m.Status.Error = m.Theme.T("memory.unavailable")
		return m, nil
	}
	if _, err := application.RefAbout(ref); err != nil {
		m.Status.Error = err.Error()
		return m, nil
	}
	m.Composer.Input.Reset()
	m.Status.Error = ""
	forget := m.deps.MemoryForget
	state := m.Header.State
	request := application.ForgetRequest{Ref: ref, Why: why, Command: command, At: time.Now(), ContextID: m.sessionContext(),
		Labels: map[string][]string{"ws": {string(state.Workspace)}, "session": {string(state.ID)}, "event": {"forget"}}}
	var receipt application.ForgetReceipt
	cmd := m.BeginOperation(func(ctx context.Context, _ *domain.Session, _ func(application.Event) error) error {
		var err error
		receipt, err = forget.Forget(ctx, request)
		return err
	})
	return m, func() tea.Msg {
		msg := cmd()
		if done, ok := msg.(operationComplete); ok && done.Err == nil {
			done.MemoryForgotten = &receipt
			return done
		}
		return msg
	}
}

// memoryCompareContent renders a comparison: the abouts with what was
// read under each, the declared relations, the proposals, and that
// nothing was written.
func memoryCompareContent(result application.CompareResult, theme Theme) string {
	var b strings.Builder
	var abouts []string
	for _, about := range result.Abouts {
		abouts = append(abouts, fmt.Sprintf("%s (%s)", about, theme.Tn("memory.entries", result.Facts[about], result.Facts[about])))
	}
	fmt.Fprintf(&b, "%s %s\n", theme.T("memory.compared"), strings.Join(abouts, " · "))
	if len(result.Declared) == 0 && len(result.Proposals) == 0 {
		b.WriteString("\n" + theme.T("memory.noRelations") + "\n")
	}
	if len(result.Declared) > 0 {
		b.WriteString("\n" + theme.T("memory.declared") + "\n")
		for _, relation := range result.Declared {
			writeRelation(&b, relation)
		}
	}
	if len(result.Proposals) > 0 {
		b.WriteString("\n" + theme.T("memory.proposals") + "\n")
		for _, relation := range result.Proposals {
			writeRelation(&b, relation)
		}
	}
	b.WriteString("\n" + theme.T("memory.readOnly"))
	return b.String()
}

func writeRelation(b *strings.Builder, relation application.MemoryRelation) {
	fmt.Fprintf(b, "- %s\n    %s %s", relation.From, relation.Rel, relation.To)
	if relation.Dimension != "" {
		fmt.Fprintf(b, " · %s=%s", relation.Dimension, singleLine(relation.Scope))
	}
	if relation.Why != "" {
		fmt.Fprintf(b, "\n    %s", singleLine(relation.Why))
	}
	b.WriteString("\n")
}

// countMemories reads the header's memory count again; nil without a
// counter or an about.
func (m AppModel) countMemories() tea.Cmd {
	counter, about := m.deps.MemoryCount, m.Header.About
	if counter == nil || about == "" {
		return nil
	}
	// The store's listing catches up with the turn's last writes a moment
	// later: counted at once on 14 October 2026, the header lagged a turn.
	return tea.Tick(memoryRecountDelay, func(time.Time) tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		count, known, err := counter.Count(ctx, about)
		if err != nil {
			return nil
		}
		return memoryCounted{about: about, count: count, known: known}
	})
}

// memoryRecountDelay is how long after a turn the header counts again.
var memoryRecountDelay = 2 * time.Second
