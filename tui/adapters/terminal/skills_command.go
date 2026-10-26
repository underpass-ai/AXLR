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

// /skills is /tools for catalog skills (docs/plans/2026-10-26-catalog-
// skills-design.md): it lists, searches, installs, updates, removes and
// publishes them, in the panel /tools uses. An install shows its card, the
// skill's origin, provenance, the tools it expects and every file, and
// waits for y, as a tool's does.

// skillsOutcome is what a /skills operation hands back to the panel.
type skillsOutcome struct {
	content string
	staged  *application.StagedSkill
	notice  string
}

// skillsCommand runs /skills with what followed it.
func (m AppModel) skillsCommand(args string) (tea.Model, tea.Cmd) {
	use := m.deps.SkillCatalog
	if !use.Enabled() {
		m.Status.Error = m.Theme.T("skills.unavailable")
		return m, nil
	}
	if m.Busy || m.knownPending() {
		m.Status.Error = m.Theme.T("mode.busy")
		return m, nil
	}
	fields := strings.Fields(args)
	verb := "list"
	if len(fields) > 0 {
		verb = strings.ToLower(fields[0])
	}
	usage := func() (tea.Model, tea.Cmd) {
		m.Status.Notice = m.Theme.Tf("notice.slashUsage", slashUsage("/skills"))
		return m, nil
	}
	workspace := string(m.Header.State.Workspace)
	var run func(ctx context.Context) (skillsOutcome, error)
	switch verb {
	case "list", "update":
		name := ""
		if len(fields) > 1 {
			name = fields[1]
		}
		run = func(ctx context.Context) (skillsOutcome, error) {
			overview, err := use.Overview(ctx, verb == "update")
			if err != nil {
				return skillsOutcome{}, err
			}
			if verb == "update" {
				for _, update := range overview.Updates {
					if name == "" || update.Name == name {
						// One card at a time: the first newer version.
						staged, err := use.Stage(ctx, application.InstallRequest{Name: update.Name, Version: update.Available, Catalog: update.Catalog})
						if err != nil {
							return skillsOutcome{}, err
						}
						return skillsOutcome{staged: &staged}, nil
					}
				}
				return skillsOutcome{notice: m.Theme.T("skills.upToDate")}, nil
			}
			return skillsOutcome{content: skillOverviewText(overview)}, nil
		}
	case "search":
		if len(fields) < 2 {
			return usage()
		}
		run = func(ctx context.Context) (skillsOutcome, error) {
			matches, _, err := use.Search(ctx, fields[1:])
			if err != nil {
				return skillsOutcome{}, err
			}
			var text strings.Builder
			for _, match := range matches {
				fmt.Fprintf(&text, "%s %s  [%s]  %s", match.Skill.Name, match.Skill.Version, match.Catalog, match.Skill.Summary)
				if len(match.Installed) > 0 {
					fmt.Fprintf(&text, "  (%s)", strings.Join(match.Installed, ", "))
				}
				text.WriteString("\n")
			}
			if len(matches) == 0 {
				text.WriteString(m.Theme.T("skills.noMatch"))
			}
			return skillsOutcome{content: text.String()}, nil
		}
	case "install":
		request, err := parseInstallArguments(fields[1:])
		if len(fields) < 2 || err != nil {
			return usage()
		}
		run = func(ctx context.Context) (skillsOutcome, error) {
			staged, err := use.Stage(ctx, request)
			if err != nil {
				return skillsOutcome{}, err
			}
			return skillsOutcome{staged: &staged}, nil
		}
	case "remove":
		if len(fields) != 2 {
			return usage()
		}
		name, version, _ := strings.Cut(fields[1], "@")
		run = func(ctx context.Context) (skillsOutcome, error) {
			removed, err := use.Remove(ctx, application.InstallRequest{Name: name, Version: version})
			if err != nil {
				return skillsOutcome{}, err
			}
			keys := make([]string, 0, len(removed))
			for _, skill := range removed {
				keys = append(keys, skill.Key())
			}
			return skillsOutcome{notice: m.Theme.Tf("skills.removed", strings.Join(keys, ", "))}, nil
		}
	case "publish":
		if len(fields) < 2 {
			return usage()
		}
		request, err := parsePublishArguments(fields[1:])
		if err != nil {
			m.Status.Error = err.Error()
			return m, nil
		}
		run = func(ctx context.Context) (skillsOutcome, error) {
			return publishSkill(ctx, use, workspace, request)
		}
	default:
		return usage()
	}
	m.Composer.Input.Reset()
	m.Info = m.themedTranscript()
	w, h := OverlayBodySize(m.Layout.Width, m.Layout.Height-1)
	m.Info.SetWidth(w)
	m.Info.Viewport.SetHeight(h)
	m.Info.SetContent(m.Theme.T("skills.loading"))
	m.Info.Viewport.GotoTop()
	m.overlay, m.toolsBrowsing, m.Status.Error = "tools", false, ""
	var outcome skillsOutcome
	cmd := m.BeginOperation(func(ctx context.Context, _ *domain.Session, _ func(application.Event) error) error {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		var err error
		outcome, err = run(ctx)
		return err
	})
	return m, skillsCompletion(cmd, &outcome)
}

// skillsCompletion attaches the outcome to the operation's completion.
func skillsCompletion(cmd tea.Cmd, outcome *skillsOutcome) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		msg := cmd()
		if done, ok := msg.(operationComplete); ok {
			done.Skills = outcome
			return done
		}
		return msg
	}
}

// completeSkills shows what the operation found, or the card to answer,
// while the panel it writes to is open.
func (m AppModel) completeSkills(outcome skillsOutcome, err error) AppModel {
	open := m.overlay == "tools" && !m.toolsBrowsing
	switch {
	case err != nil:
		m.Status.Error = err.Error()
		if open {
			m.Info.SetContent(err.Error())
		}
	case outcome.staged != nil && open:
		m.skillsStaged, m.skillsAsking = outcome.staged, true
		m.Info.SetContent(skillCardText(*outcome.staged))
		m.Status.Notice = m.Theme.Tf("skills.confirm", outcome.staged.Entry.Name, outcome.staged.Entry.Version, outcome.staged.Catalog)
	case outcome.content != "" && open:
		m.Info.SetContent(outcome.content)
	case open:
		m.overlay = ""
	}
	if outcome.notice != "" {
		m.Status.Notice = outcome.notice
	}
	m.Info.Viewport.GotoTop()
	return m
}

// answerSkills takes the key that answers an install card: y installs,
// any other key keeps the card's skill out; the card scrolls first.
func (m AppModel) answerSkills(k tea.KeyPressMsg) (AppModel, tea.Cmd) {
	switch k.String() {
	case "up", "down", "pgup", "pgdown", "home", "end", "ctrl+home", "ctrl+end", "ctrl+u", "ctrl+d", "ctrl+f", "ctrl+b":
		if !jumpViewport(&m.Info.Viewport, k.String()) {
			m.Info.Viewport, _ = m.Info.Viewport.Update(k)
		}
		return m, nil
	}
	staged := m.skillsStaged
	m.skillsAsking, m.skillsStaged = false, nil
	if k.String() != "y" || staged == nil {
		m.Status.Notice = m.Theme.T("tools.kept")
		m.overlay = ""
		return m, nil
	}
	use := m.deps.SkillCatalog
	var outcome skillsOutcome
	cmd := m.BeginOperation(func(ctx context.Context, _ *domain.Session, _ func(application.Event) error) error {
		installed, _, err := use.Install(ctx, *staged, time.Now())
		if err != nil {
			return err
		}
		outcome.notice = m.Theme.Tf("skills.installed", installed.Key())
		return nil
	})
	return m, skillsCompletion(cmd, &outcome)
}

// publishSkill signs and uploads a workspace skill and waits for the
// registry's scan and checks, as /tools publish does without its
// questions: a skill's catalog.json must be complete.
func publishSkill(ctx context.Context, use application.SkillCatalogUseCase, workspace string, request application.PublishRequest) (skillsOutcome, error) {
	draft, err := use.PrepareSkill(ctx, workspace, request)
	if err != nil {
		return skillsOutcome{}, err
	}
	bundle, err := use.BundleSkill(ctx, draft)
	if err != nil {
		return skillsOutcome{}, err
	}
	report, err := use.SubmitSkill(ctx, draft, bundle)
	if err != nil {
		return skillsOutcome{}, err
	}
	deadline := time.Now().Add(application.PublishPollDeadline)
	for !report.Settled() && time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return skillsOutcome{}, ctx.Err()
		case <-time.After(application.PublishPollInterval):
		}
		if report, err = use.PollSkill(ctx, draft); err != nil {
			return skillsOutcome{}, err
		}
	}
	var text strings.Builder
	fmt.Fprintf(&text, "%s %s → catalog %s (%s)\nbundle %s, key %s\nstatus %s", draft.Name, draft.Metadata.Version, draft.Catalog, draft.API, bundle.Digest, bundle.KeyID, report.Status)
	if report.TestsPassed != nil {
		fmt.Fprintf(&text, ", checks passed: %t", *report.TestsPassed)
	}
	for _, failed := range report.FailedTests {
		fmt.Fprintf(&text, "\nfailed: %s", failed)
	}
	if report.RejectedReason != "" {
		fmt.Fprintf(&text, "\nrejected: %s", report.RejectedReason)
	}
	return skillsOutcome{content: text.String()}, nil
}

func skillOverviewText(overview application.SkillOverview) string {
	var text strings.Builder
	for _, state := range overview.Catalogs {
		if state.Warning != "" {
			fmt.Fprintf(&text, "catalog %s: %s\n", state.Name, state.Warning)
		}
	}
	text.WriteString("Installed\n")
	if len(overview.Installed) == 0 {
		text.WriteString("  —\n")
	}
	for _, skill := range overview.Installed {
		fmt.Fprintf(&text, "  %s  %s\n", skill.Key(), skill.Description)
	}
	for _, state := range overview.Catalogs {
		if state.Index == nil || len(state.Index.Skills) == 0 {
			continue
		}
		fmt.Fprintf(&text, "\nCatalog %s\n", state.Name)
		for _, skill := range state.Index.Skills {
			fmt.Fprintf(&text, "  %s %s  %s\n", skill.Name, skill.Version, skill.Summary)
		}
	}
	for _, update := range overview.Updates {
		fmt.Fprintf(&text, "\n↑ %s %s → %s (%s): /skills update %s", update.Name, update.Installed, update.Available, update.Catalog, update.Name)
	}
	return text.String()
}

// skillCardText is the install card: origin, provenance, the tools the
// skill expects and which this console lacks, and every file as it will be
// installed.
func skillCardText(staged application.StagedSkill) string {
	var text strings.Builder
	fmt.Fprintf(&text, "%s %s · catalog %s · verified (index %s)\n", staged.Entry.Name, staged.Entry.Version, staged.Catalog, staged.IndexGeneratedAt)
	if staged.Entry.Publisher != "" {
		fmt.Fprintf(&text, "publisher %s · reviewer %s · bundle %s\n", staged.Entry.Publisher, staged.Entry.Reviewer, staged.Entry.BundleSHA256)
	}
	fmt.Fprintf(&text, "%s\n", staged.Description)
	if len(staged.Tools) > 0 {
		fmt.Fprintf(&text, "tools: %s\n", strings.Join(staged.Tools, ", "))
	}
	if len(staged.Missing) > 0 {
		fmt.Fprintf(&text, "not offered here: %s\n", strings.Join(staged.Missing, ", "))
	}
	for _, warning := range staged.Warnings {
		fmt.Fprintf(&text, "warning: %s\n", warning)
	}
	for _, file := range staged.Files {
		fmt.Fprintf(&text, "\n──── %s (%d bytes)\n%s", file.Path, len(file.Content), file.Content)
	}
	return text.String()
}
