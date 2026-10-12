package terminal

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func writeTree(t *testing.T, root string, files ...string) {
	t.Helper()
	for _, name := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTheWorkspaceListingSkipsDependenciesAndIgnoredFiles(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, "main.go", "tui/app_model.go", "node_modules/x/index.js", ".git/config", "build/out.bin", "debug.log", "docs/console.md", "gen/keep.txt")
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("# comment\nbuild/\n*.log\n!keep.txt\n/gen/*.tmp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Symlink(filepath.Join(root, "tui"), filepath.Join(root, "linked")); err != nil {
			t.Fatal(err)
		}
	}
	files, truncated, err := listWorkspaceFiles(root)
	if err != nil || truncated {
		t.Fatalf("walk: %v truncated %v", err, truncated)
	}
	got := strings.Join(files, ",")
	for _, want := range []string{"main.go", "tui/", "tui/app_model.go", "docs/console.md", "gen/keep.txt", ".gitignore"} {
		if !strings.Contains(","+got+",", ","+want+",") {
			t.Errorf("listing lacks %s: %s", want, got)
		}
	}
	for _, unwanted := range []string{"node_modules", ".git/", "build", "debug.log", "linked/"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("listing has %s: %s", unwanted, got)
		}
	}
	if ranked := rankFiles(files, "app", 10); len(ranked) == 0 || ranked[0] != "tui/app_model.go" {
		t.Fatalf("ranked %v", ranked)
	}
	if ranked := rankFiles([]string{"docs/console.md", "tui/adapters/terminal/console_note_row.go", "consolex.go"}, "console", 10); strings.Join(ranked, ",") != "consolex.go,docs/console.md,tui/adapters/terminal/console_note_row.go" {
		t.Fatalf("ranked by name first, shorter first: %v", ranked)
	}
	if ranked := rankFiles([]string{"tui/app_model.go"}, "tapm", 10); len(ranked) != 1 {
		t.Fatalf("letters in order did not match: %v", ranked)
	}
}

// On 12 October 2026 "@buil" + Tab did nothing: every prompt spelt its
// paths. @ opens a menu of the workspace's files; Tab or Enter inserts the
// path, Esc closes it.
func TestAtOpensAFileMenuAndTabInsertsThePath(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, "buildinfo/version.go", "main.go")
	s, err := domain.NewSession("0123456789abcdef0123456789abcdef", domain.Workspace(root), "model")
	if err != nil {
		t.Fatal(err)
	}
	m := update(New(Dependencies{Session: &s, Monochrome: true}), tea.WindowSizeMsg{Width: 100, Height: 30})
	defer m.zones.Close()
	m = typeDraft(m, "look at ")
	var cmd tea.Cmd
	for _, r := range "@versi" {
		var next tea.Model
		next, cmd = m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		m = next.(AppModel)
		if r == '@' {
			if cmd == nil {
				t.Fatal("@ did not list the workspace")
			}
			msg := cmd()
			if batch, ok := msg.(tea.BatchMsg); ok {
				for _, c := range batch {
					if c != nil {
						if files, ok := c().(workspaceFilesMsg); ok {
							msg = files
						}
					}
				}
			}
			m = update(m, msg)
		}
	}
	view := ansi.Strip(m.View().Content)
	if !strings.Contains(view, "> buildinfo/version.go") {
		t.Fatalf("the menu does not offer the file:\n%s", view)
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyTab})
	if got := m.Composer.Input.Value(); got != "look at buildinfo/version.go " {
		t.Fatalf("Tab inserted %q", got)
	}
	m = typeDraft(m, "and @ma")
	if !m.mentionActive() {
		t.Fatal("a second @ did not open the menu")
	}
	m = update(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.mentionActive() || m.Composer.Input.Value() != "look at buildinfo/version.go and @ma" {
		t.Fatalf("Esc did not close the menu: draft %q", m.Composer.Input.Value())
	}
	// In the middle of the draft the path replaces the word and the rest
	// stays after it.
	m.Composer.Input.SetValue("see @mai please")
	m.Composer.Input.SetCursorColumn(len("see @mai"))
	m.insertMention("main.go")
	if got := m.Composer.Input.Value(); got != "see main.go please" || m.Composer.Input.Column() != len("see main.go") {
		t.Fatalf("inserted mid-draft: %q at %d", got, m.Composer.Input.Column())
	}
	m.Composer.Input.SetValue("first @mai\nsecond line")
	m.Composer.Input.MoveToBegin()
	m.Composer.Input.SetCursorColumn(len("first @mai"))
	m.insertMention("main.go")
	if got := m.Composer.Input.Value(); got != "first main.go \nsecond line" || m.Composer.Input.Line() != 0 || m.Composer.Input.Column() != len("first main.go ") {
		t.Fatalf("inserted on the first line: %q at %d:%d", got, m.Composer.Input.Line(), m.Composer.Input.Column())
	}
	// A turn's end lists the files again the next time.
	m.mention.stale = true
	if m.loadFiles(false) == nil {
		t.Fatal("a stale listing was not refreshed")
	}
}
