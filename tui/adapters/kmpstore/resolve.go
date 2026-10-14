package kmpstore

import (
	"os"
	"path/filepath"
	"strings"
)

// UserDefault is KMP's per-user default store under the data directory.
func UserDefault(dataBase string) string { return filepath.Join(dataBase, "kmp", "default") }

// Resolve chooses the store for a workspace from the registration's
// environment: a pinned directory other than the per-user default is the
// operator's and wins; the installer's pin of the per-user default (what it
// wrote until 13 October 2026) is replaced by the checkout's .kernel when
// the workspace is inside a git repository; outside one the per-user default
// is used.
func Resolve(workspace string, env []string, dataBase string) Selection {
	pinned := Value(env, DataDirVariable)
	user := UserDefault(dataBase)
	if pinned != "" && filepath.Clean(pinned) != filepath.Clean(user) {
		return Selection{Path: pinned, Reason: ReasonOperator}
	}
	if top, ok := GitTopLevel(workspace); ok {
		return Selection{Path: filepath.Join(top, StoreDirectory), Reason: ReasonProject, TopLevel: top, Replaced: pinned}
	}
	return Selection{Path: user, Reason: ReasonUser}
}

// GitTopLevel is the project root of a workspace, found the way KMP finds
// it: the nearest directory at or above the workspace that is a git
// checkout or already holds a store. A worktree (whose .git is a file
// pointing into the main repository's .git/worktrees) resolves to the main
// repository, so worktrees share its store; a submodule's .git file keeps
// the submodule as its own project.
func GitTopLevel(workspace string) (string, bool) {
	dir := filepath.Clean(workspace)
	for {
		if Exists(filepath.Join(dir, StoreDirectory)) {
			return dir, true
		}
		if info, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			if info.IsDir() {
				return dir, true
			}
			if info.Mode().IsRegular() {
				if main, ok := worktreeMain(dir); ok {
					return main, true
				}
				return dir, true
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// worktreeMain reads a worktree's .git file, "gitdir: <main>/.git/worktrees/<name>",
// and returns <main>; false for any other .git file.
func worktreeMain(dir string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(dir, ".git"))
	if err != nil {
		return "", false
	}
	line, _, _ := strings.Cut(string(data), "\n")
	target, ok := strings.CutPrefix(strings.TrimSpace(line), "gitdir:")
	if !ok {
		return "", false
	}
	target = filepath.Clean(strings.TrimSpace(target))
	if !filepath.IsAbs(target) {
		target = filepath.Join(dir, target)
	}
	// <main>/.git/worktrees/<name>
	worktrees := filepath.Dir(target)
	gitDir := filepath.Dir(worktrees)
	if filepath.Base(worktrees) != "worktrees" || filepath.Base(gitDir) != ".git" {
		return "", false
	}
	return filepath.Dir(gitDir), true
}
