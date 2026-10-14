package terminal

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
)

// The emblems drawn are the ones the README shows on GitHub.
func TestAxlrInfoEmblemsMatchTheBrandAssets(t *testing.T) {
	for name, embedded := range map[string]string{"axlr-emblem-dark.svg": emblemDarkSVG, "axlr-emblem-light.svg": emblemLightSVG} {
		want, err := os.ReadFile("../../../docs/assets/brand/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if string(want) != embedded {
			t.Fatalf("brand/%s differs from docs/assets/brand/%s: copy it again", name, name)
		}
	}
	for _, logo := range []emblem{emblemDark, emblemLight} {
		if len(logo) != 14 || len(logo[0]) != 67 {
			t.Fatalf("emblem is %dx%d", len(logo), len(logo[0]))
		}
	}
}

type versionsStub struct{ versions map[string]string }

func (versionsStub) Update(context.Context) ([]application.EngineUpdateResult, error) {
	return nil, nil
}
func (v versionsStub) InstalledVersions(context.Context) (map[string]string, error) {
	return v.versions, nil
}

// drive executes cmd and its batches, feeding every message back, until
// nothing more comes or limit messages were delivered.
func drive(t *testing.T, m AppModel, cmd tea.Cmd, limit int) (AppModel, int) {
	t.Helper()
	frame := axlrInfoFrame
	axlrInfoFrame = time.Microsecond
	defer func() { axlrInfoFrame = frame }()
	queue, delivered := []tea.Cmd{cmd}, 0
	for len(queue) > 0 && delivered < limit {
		next := queue[0]
		queue = queue[1:]
		if next == nil {
			continue
		}
		msg := next()
		if batch, ok := msg.(tea.BatchMsg); ok {
			queue = append(queue, batch...)
			continue
		}
		delivered++
		changed, more := m.Update(msg)
		m = changed.(AppModel)
		queue = append(queue, more)
	}
	return m, delivered
}

func TestAxlrInfoAnimatesSettlesAndShowsVersions(t *testing.T) {
	m := sized()
	m.deps.EngineUpdates = versionsStub{versions: map[string]string{"kmp": "1.4.2"}}
	m.deps.Jev = JevInfo{Model: "jev-1.13.0", Tool: true, FinalCheck: true, Threshold: 0.5}
	m.Header.State.Model = "z-ai/glm-5.3-flash"
	m.Composer.Input.SetValue("/axlr-info")
	next, cmd := m.Update(ControlIntent("send"))
	m = next.(AppModel)
	if m.overlay != "axlr-info" || m.Composer.Input.Value() != "" || cmd == nil {
		t.Fatalf("overlay=%q draft=%q cmd=%v", m.overlay, m.Composer.Input.Value(), cmd != nil)
	}
	early := ansi.Strip(m.View().Content)
	m, delivered := drive(t, m, cmd, 2000)
	if delivered >= 2000 {
		t.Fatal("the animation never settles")
	}
	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"█", "Execute with AXLR. Remember with KMP. Coordinate with MADE.", "AXLR 0.0.0-dev", "github.com/underpass-ai/AXLR", "KMP 1.4.2", "MADE not installed", "z-ai/glm-5.3-flash via OpenRouter", "Jev on · jev-1.13.0 · axlr_judge tool + final check below 50%", "esc close"} {
		if !strings.Contains(view, want) {
			t.Fatalf("settled screen lacks %q:\n%s", want, view)
		}
	}
	if early == view {
		t.Fatal("the first frame is the last: nothing moved")
	}
	// A key is the screen's, never the composer's.
	m = update(m, tea.KeyPressMsg{Code: 'x', Text: "x"})
	if m.Composer.Input.Value() != "" || m.overlay != "axlr-info" {
		t.Fatalf("x reached the composer (%q) or closed (%q)", m.Composer.Input.Value(), m.overlay)
	}
	// r replays; Esc closes and a late tick does not bring it back.
	next, cmd = m.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	m = next.(AppModel)
	if cmd == nil || m.AxlrInfo.frame != 0 {
		t.Fatalf("r did not replay: frame %d", m.AxlrInfo.frame)
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.overlay != "" {
		t.Fatalf("esc left %q", m.overlay)
	}
	m, _ = drive(t, m, cmd, 50)
	if m.overlay != "" {
		t.Fatalf("a late tick reopened %q", m.overlay)
	}
}

func TestAxlrInfoSaysJevOffAndALocalModel(t *testing.T) {
	m := sized()
	m.Header.State.Model = "qwen-local"
	m.deps.LocalEndpoints = map[root.ModelID]string{"qwen-local": "http://127.0.0.1:8080/v1"}
	m.Theme.Locale = Spanish
	m, cmd := m.openAxlrInfo()
	m, _ = drive(t, m, cmd, 2000)
	view := ansi.Strip(m.View().Content)
	for _, want := range []string{"qwen-local vía servidor local http://127.0.0.1:8080/v1", "Jev desactivado", "Ejecuta con AXLR."} {
		if !strings.Contains(view, want) {
			t.Fatalf("screen lacks %q:\n%s", want, view)
		}
	}
}
