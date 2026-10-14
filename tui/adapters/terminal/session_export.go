package terminal

import (
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// exportSession writes the open session as Markdown, to path or, without
// one, to <workspace>/.axlr/exports/, and says where in the footer. On 12
// October 2026 /copy, which copies the last reply, was the only way to
// take a conversation out of the console. A relative path is the
// workspace's; "~/" is the home directory.
func (m AppModel) exportSession(path string) (tea.Model, tea.Cmd) {
	if m.deps.ExportFiles == nil {
		m.Status.Error = m.Theme.T("export.unavailable")
		return m, nil
	}
	path = strings.TrimSpace(path)
	if rest, ok := strings.CutPrefix(path, "~/"); ok {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			path = filepath.Join(home, rest)
		}
	}
	export := application.ExportSessionUseCase{Files: m.deps.ExportFiles, Usage: m.deps.Usage, Labels: m.deps.SessionLabels}
	written, err := export.Execute(m.lifetime.ctx, m.Header.State, path)
	if err != nil {
		m.Status.Notice = ""
		m.Status.Error = m.Theme.Tf("export.failed", err.Error())
		return m, nil
	}
	m.Composer.Input.Reset()
	m.Status.Error = ""
	m.Status.Notice = m.Theme.Tf("export.done", shortenHome(written))
	return m, nil
}

// exportSessionBundle writes the open session, or the saved one id names,
// as a bundle another console imports with --import (decision I4 of the
// lifecycle integration design).
func (m AppModel) exportSessionBundle(id string) (tea.Model, tea.Cmd) {
	if m.deps.SessionBundles == nil || m.deps.ExportFiles == nil {
		m.Status.Error = m.Theme.T("export.unavailable")
		return m, nil
	}
	var open domain.Session
	if m.deps.Session != nil {
		open = *m.deps.Session
	}
	target := open.ID()
	if id = strings.TrimSpace(id); id != "" {
		parsed, err := domain.NewSessionID(id)
		if err != nil {
			m.Status.Error = err.Error()
			return m, nil
		}
		target = parsed
	}
	export := application.ExportSessionBundleUseCase{Codec: m.deps.SessionBundles, Files: m.deps.ExportFiles, Labels: m.deps.SessionLabels, Sessions: m.deps.Sessions, Host: m.deps.Hostname}
	written, err := export.Execute(m.lifetime.ctx, open, target)
	if err != nil {
		m.Status.Notice = ""
		m.Status.Error = m.Theme.Tf("export.failed", err.Error())
		return m, nil
	}
	m.Composer.Input.Reset()
	m.Status.Error = ""
	m.Status.Notice = m.Theme.Tf("export.done", shortenHome(written))
	return m, nil
}
