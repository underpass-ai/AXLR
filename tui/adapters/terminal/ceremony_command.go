package terminal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// /ceremony (decisions H3 and H4 of the lifecycle integration design):
// alone it lists the published definitions; with a definition (and a
// version, else the newest) it arms the generic loop for the next prompt,
// or runs the brief given after it; "publish <file.yaml>" publishes a
// definition the person wrote, through the console's install grant, as P
// in /mcp does for the console's own.

var versionWord = regexp.MustCompile(`^\d+(\.\d+)*$`)

// ceremonyCommand runs /ceremony with what followed it.
func (m AppModel) ceremonyCommand(args string) (tea.Model, tea.Cmd) {
	args = strings.TrimSpace(args)
	if m.deps.Definitions == nil {
		m.Status.Error = m.Theme.T("ceremonies.unavailable")
		return m, nil
	}
	if args == "" {
		if m.Busy {
			m.Status.Error = m.Theme.T("mode.busy")
			return m, nil
		}
		m.Composer.Input.Reset()
		return m.openCeremoniesPanel()
	}
	fields := strings.Fields(args)
	if strings.EqualFold(fields[0], "publish") {
		if len(fields) < 2 {
			m.Status.Notice = m.Theme.Tf("notice.slashUsage", slashUsage("/ceremony"))
			return m, nil
		}
		return m.publishCeremony(strings.TrimSpace(strings.TrimPrefix(args, fields[0])))
	}
	if m.Busy || m.knownPending() {
		m.Status.Error = m.Theme.T("mode.busy")
		return m, nil
	}
	name, rest := fields[0], fields[1:]
	version := ""
	if len(rest) > 0 && versionWord.MatchString(rest[0]) {
		version, rest = rest[0], rest[1:]
	}
	// The list is read off the screen's thread: on 14 October 2026 review
	// found /ceremony freezing the console for MADE's answer (up to 15 s).
	arm := ceremonyArm{name: name, version: version, brief: strings.TrimSpace(strings.Join(rest, " "))}
	m.Status.Error = ""
	m.Status.Notice = m.Theme.T("ceremonies.loading")
	return m, m.listCeremonies(&arm)
}

// listCeremonies reads the published definitions in an operation; the
// completion fills the panel, or arms the definition arm names.
func (m *AppModel) listCeremonies(arm *ceremonyArm) tea.Cmd {
	definitions := m.deps.Definitions
	var listed []application.CeremonyDefinitionSummary
	cmd := m.BeginOperation(func(ctx context.Context, _ *domain.Session, _ func(application.Event) error) error {
		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		var err error
		listed, err = definitions.List(ctx)
		return err
	})
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		msg := cmd()
		if done, ok := msg.(operationComplete); ok {
			done.Ceremonies = &listed
			done.CeremonyArm = arm
			return done
		}
		return msg
	}
}

// armCeremony chooses the definition arm names from listed for the next
// prompt, and sends the brief given with it.
func (m AppModel) armCeremony(listed []application.CeremonyDefinitionSummary, arm ceremonyArm) (tea.Model, tea.Cmd) {
	if m.Busy || m.knownPending() {
		m.Status.Error = m.Theme.T("mode.busy")
		return m, nil
	}
	name, version := arm.name, arm.version
	if version == "" {
		version = application.LatestVersion(listed, name)
	}
	known, named := false, false
	var versions []string
	for _, item := range listed {
		if item.Name == name {
			named = true
			versions = append(versions, item.Version)
		}
		if item.Name == name && item.Version == version {
			known = true
		}
	}
	if !known {
		m.Status.Notice = ""
		m.Status.Error = m.Theme.Tf("error.ceremonyUnknown", name)
		if named {
			m.Status.Error = m.Theme.Tf("error.ceremonyVersionUnknown", version, name, strings.Join(versions, ", "))
		}
		return m, nil
	}
	if m.deps.Session == nil || m.deps.Store == nil {
		m.Status.Error = m.Theme.T("mode.unavailable")
		return m, nil
	}
	next := *m.deps.Session
	if err := next.ChooseCeremony(name, version); err != nil {
		m.Status.Error = err.Error()
		return m, nil
	}
	if err := m.deps.Store.Save(m.lifetime.ctx, next); err != nil {
		m.Status.Error = err.Error()
		return m, nil
	}
	*m.deps.Session = next
	m.Header.State = next.Export()
	m.Status.Error = ""
	m.Composer.Input.Reset()
	if arm.brief != "" {
		m.Composer.Input.SetValue(arm.brief)
		return m.submitDraft()
	}
	m.Status.Notice = m.Theme.Tf("notice.ceremonyArmed", name+" "+version)
	return m, nil
}

// openCeremoniesPanel opens the list of published definitions; the
// operation that reads it fills the panel.
func (m AppModel) openCeremoniesPanel() (AppModel, tea.Cmd) {
	m.Info = NewTranscript()
	w, h := OverlayBodySize(m.Layout.Width, m.Layout.Height-1)
	m.Info.SetWidth(w)
	m.Info.Viewport.SetHeight(h)
	m.Info.SetContent(m.Theme.T("ceremonies.loading"))
	m.Info.Viewport.GotoTop()
	m.overlay = "ceremonies"
	cmd := m.listCeremonies(nil)
	return m, cmd
}

// ceremoniesContent renders the published definitions for the panel.
func ceremoniesContent(listed []application.CeremonyDefinitionSummary, err error, theme Theme) string {
	var b strings.Builder
	switch {
	case err != nil:
		b.WriteString(err.Error())
	case len(listed) == 0:
		b.WriteString(theme.T("ceremonies.none"))
	default:
		for _, item := range listed {
			fmt.Fprintf(&b, "%s %s · %s\n  %s\n", item.Name, item.Version, theme.Tf("ceremonies.steps", item.Steps), singleLine(item.Description))
		}
		b.WriteString("\n" + theme.T("ceremonies.hint"))
	}
	return b.String()
}

// publishCeremony validates and publishes a definition file through the
// console's install grant; the completion shows what was published.
func (m AppModel) publishCeremony(path string) (tea.Model, tea.Cmd) {
	if m.deps.OrchestratorPreparation == nil {
		m.Status.Error = m.Theme.T("ceremonies.unavailable")
		return m, nil
	}
	if m.Busy {
		m.Status.Error = m.Theme.T("mode.busy")
		return m, nil
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(string(m.deps.Workspace), path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		m.Status.Error = err.Error()
		return m, nil
	}
	m.Composer.Input.Reset()
	m.Status.Error = ""
	preparer := m.deps.OrchestratorPreparation
	text := string(data)
	var published string
	cmd := m.BeginOperation(func(ctx context.Context, _ *domain.Session, _ func(application.Event) error) error {
		var err error
		published, err = preparer.Publish(ctx, text)
		return err
	})
	return m, func() tea.Msg {
		msg := cmd()
		if done, ok := msg.(operationComplete); ok {
			done.Published = published
			return done
		}
		return msg
	}
}
