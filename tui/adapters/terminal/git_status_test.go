package terminal

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// The branch comes from HEAD, also through a worktree's .git file, and the
// dirty marker from git when it is installed.
func TestReadGitState(t *testing.T) {
	dir := t.TempDir()
	if _, err := ReadGitState(context.Background(), domain.Workspace(dir)); err == nil {
		t.Fatal("a directory outside Git has a branch")
	}
	gitDir := filepath.Join(dir, "repo-git")
	if err := os.MkdirAll(gitDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/claude/qa3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: repo-git\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if state, err := ReadGitState(context.Background(), domain.Workspace(dir)); err != nil || state.Branch != "claude/qa3" {
		t.Fatalf("worktree: %+v %v", state, err)
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	repo := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo, "-c", "user.email=a@b", "-c", "user.name=a"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run("add", "a.txt")
	run("commit", "-q", "-m", "one")
	if state, err := ReadGitState(context.Background(), domain.Workspace(repo)); err != nil || state != (GitState{Branch: "main"}) {
		t.Fatalf("clean: %+v %v", state, err)
	}
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if state, err := ReadGitState(context.Background(), domain.Workspace(repo)); err != nil || state != (GitState{Branch: "main", Dirty: true}) {
		t.Fatalf("dirty: %+v %v", state, err)
	}
}

// The header asks at launch and after each turn, and shows what it got.
func TestTheHeaderShowsTheBranch(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	asked := 0
	m.deps.GitState = func(context.Context, domain.Workspace) (GitState, error) {
		asked++
		return GitState{Branch: "main", Dirty: true}, nil
	}
	m = update(m, m.readGitState()())
	if m.Header.Git == nil || asked != 1 || !strings.Contains(m.View().Content, "git: main*") {
		t.Fatalf("header %q", m.Header.View(100, m.Theme))
	}
	m.Busy, m.operationID, m.providerWaiting = true, 1, true
	_, cmd := m.Update(operationComplete{ID: 1, Session: *m.deps.Session})
	runCmds(cmd)
	if asked != 2 {
		t.Fatalf("a turn's end asked %d times in all", asked)
	}
}

// runCmds runs a command and, for a batch, each of its commands once.
func runCmds(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	if batch, ok := cmd().(tea.BatchMsg); ok {
		for _, c := range batch {
			runCmds(c)
		}
	}
}
