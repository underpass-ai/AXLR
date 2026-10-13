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
		m.Composer.Input.Reset()
		return m.openCeremoniesPanel(), nil
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
	ctx, cancel := context.WithTimeout(m.lifetime.ctx, 15*time.Second)
	defer cancel()
	listed, err := m.deps.Definitions.List(ctx)
	if err != nil {
		m.Status.Error = err.Error()
		return m, nil
	}
	if version == "" {
		version = application.LatestVersion(listed, name)
	}
	known := false
	for _, item := range listed {
		if item.Name == name && item.Version == version {
			known = true
		}
	}
	if !known {
		m.Status.Error = m.Theme.Tf("error.ceremonyUnknown", name)
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
	if brief := strings.TrimSpace(strings.Join(rest, " ")); brief != "" {
		m.Composer.Input.SetValue(brief)
		return m.submitDraft()
	}
	m.Status.Notice = m.Theme.Tf("notice.ceremonyArmed", name+" "+version)
	return m, nil
}

// openCeremoniesPanel lists the published definitions.
func (m AppModel) openCeremoniesPanel() AppModel {
	ctx, cancel := context.WithTimeout(m.lifetime.ctx, 15*time.Second)
	defer cancel()
	listed, err := m.deps.Definitions.List(ctx)
	var b strings.Builder
	switch {
	case err != nil:
		b.WriteString(err.Error())
	case len(listed) == 0:
		b.WriteString(m.Theme.T("ceremonies.none"))
	default:
		for _, item := range listed {
			fmt.Fprintf(&b, "%s %s · %s\n  %s\n", item.Name, item.Version, m.Theme.Tf("ceremonies.steps", item.Steps), singleLine(item.Description))
		}
		b.WriteString("\n" + m.Theme.T("ceremonies.hint"))
	}
	m.Info = NewTranscript()
	w, h := OverlayBodySize(m.Layout.Width, m.Layout.Height-1)
	m.Info.SetWidth(w)
	m.Info.Viewport.SetHeight(h)
	m.Info.SetContent(b.String())
	m.Info.Viewport.GotoTop()
	m.overlay = "ceremonies"
	return m
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
