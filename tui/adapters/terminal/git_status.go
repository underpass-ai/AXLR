package terminal

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// GitState is the workspace's branch for the header, "⎇ main*": the branch
// (or a short commit when detached) and whether tracked files differ from
// it. A workspace outside Git has no branch.
type GitState struct {
	Branch string
	Dirty  bool
}

// gitStatusTimeout bounds the dirty check: the header goes without the
// marker rather than wait.
const gitStatusTimeout = 3 * time.Second

// ReadGitState reads a workspace's branch from its HEAD file, following a
// worktree's or submodule's ".git" file, and asks git whether tracked files
// changed. It takes no lock (GIT_OPTIONAL_LOCKS=0), so the model's own git
// commands never wait for it, and runs no fsmonitor hook a repository may
// configure. Without git, or past the timeout, the branch comes alone.
func ReadGitState(ctx context.Context, workspace domain.Workspace) (GitState, error) {
	dir, err := gitDir(string(workspace))
	if err != nil {
		return GitState{}, err
	}
	head, err := os.ReadFile(filepath.Join(dir, "HEAD"))
	if err != nil {
		return GitState{}, err
	}
	state := GitState{}
	ref := strings.TrimSpace(string(head))
	if branch, ok := strings.CutPrefix(ref, "ref: refs/heads/"); ok {
		state.Branch = branch
	} else if len(ref) >= 7 {
		state.Branch = ref[:7]
	} else {
		return GitState{}, errors.New("unreadable HEAD")
	}
	ctx, cancel := context.WithTimeout(ctx, gitStatusTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-c", "core.fsmonitor=false", "-C", string(workspace), "status", "--porcelain", "--untracked-files=no")
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	if out, err := cmd.Output(); err == nil {
		state.Dirty = len(bytes.TrimSpace(out)) > 0
	}
	return state, nil
}

// gitDir is the repository directory of a workspace at its root: ".git",
// or where a ".git" file points.
func gitDir(workspace string) (string, error) {
	path := filepath.Join(workspace, ".git")
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return path, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	line, _ := bufio.NewReader(file).ReadString('\n')
	target, ok := strings.CutPrefix(strings.TrimSpace(line), "gitdir:")
	if !ok {
		return "", errors.New("unreadable .git file")
	}
	target = strings.TrimSpace(target)
	if !filepath.IsAbs(target) {
		target = filepath.Join(workspace, target)
	}
	return target, nil
}

// gitStateMsg carries a workspace's Git state to the header.
type gitStateMsg struct {
	Workspace domain.Workspace
	State     GitState
	Err       error
}

// readGitState asks for the workspace's Git state in the background: at
// launch and after each turn, which may have committed or switched.
func (m AppModel) readGitState() tea.Cmd {
	read, workspace := m.deps.GitState, m.Header.State.Workspace
	if read == nil || workspace == "" {
		return nil
	}
	ctx := m.lifetime.ctx
	return func() tea.Msg {
		state, err := read(ctx, workspace)
		return gitStateMsg{Workspace: workspace, State: state, Err: err}
	}
}
