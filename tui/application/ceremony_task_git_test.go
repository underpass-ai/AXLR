package application

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// gitDirChecks runs commands in a real directory the way ceremonyhost.Checks
// answers them: Output is the 4 KiB tail of stdout and stderr with a leading
// "…", Stdout the whole stdout.
type gitDirChecks struct{ dir string }

func (c gitDirChecks) Run(ctx context.Context, command domain.CheckCommand) (CheckResult, error) {
	cmd := exec.CommandContext(ctx, command.Program, command.Args...)
	cmd.Dir = c.dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		return CheckResult{ExitCode: -1, Output: err.Error()}, nil
	}
	output := stdout.String() + stderr.String()
	if len(output) > 4<<10 {
		cut := len(output) - 4<<10
		for cut < len(output) && !utf8Start(output[cut]) {
			cut++
		}
		output = "…" + output[cut:]
	}
	return CheckResult{Ran: true, ExitCode: code, Output: output, Stdout: stdout.String()}, nil
}

// gitDirFiles reads the same directory for the task's digests.
type gitDirFiles struct{ dir string }

func (f gitDirFiles) Read(_ context.Context, path string, max int) ([]byte, bool, error) {
	content, err := os.ReadFile(filepath.Join(f.dir, path))
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if len(content) > max {
		content = content[:max]
	}
	return content, err == nil, err
}
func (gitDirFiles) Write(context.Context, string, []byte) error { return nil }
func (gitDirFiles) MakeDir(context.Context, string) error       { return nil }

// gitRepository makes a repository with one commit of the given files.
func gitRepository(t *testing.T, files map[string]string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	for name, content := range files {
		writeFile(t, dir, name, content)
	}
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "t@t"}, {"config", "user.name", "t"}, {"add", "-A"}, {"commit", "-q", "--allow-empty", "-m", "init"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return dir
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A task that changes a file outside its scope must be caught however long
// git status is: earlier waves may leave many new files, and the scope check
// read only the 4 KiB tail meant for the model, so the first paths were lost
// and the cut first line became a path that does not exist.
func TestTaskScopeCheckSeesEveryChangeInALongGitStatus(t *testing.T) {
	dir := gitRepository(t, map[string]string{".github/ci.yml": "a\n"})
	for i := 0; i < 150; i++ {
		writeFile(t, dir, fmt.Sprintf("testdata/golden/case-%03d.json", i), "{}")
	}
	d := &CeremonyDriver{Checks: gitDirChecks{dir: dir}, Files: gitDirFiles{dir: dir}}
	task := &domain.TaskRun{Scope: []string{"lines.go"}}
	start, err := d.taskDigests(context.Background(), task)
	if err != nil || !task.Git {
		t.Fatalf("digests: git=%v err=%v", task.Git, err)
	}
	task.Start = start
	// The task tampers with CI, which sorts before every earlier change.
	writeFile(t, dir, ".github/ci.yml", "tampered\n")
	paths, _, err := d.gitChanged(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 151 || !slices.Contains(paths, ".github/ci.yml") {
		t.Fatalf("git status lists %d paths, .github/ci.yml among them: %v", len(paths), slices.Contains(paths, ".github/ci.yml"))
	}
	for _, p := range paths {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Fatalf("gitChanged invented the path %q", p)
		}
	}
	changed, err := d.changedByTask(context.Background(), task)
	if err != nil || strings.Join(changed, ",") != ".github/ci.yml" {
		t.Fatalf("the task changed %q (err %v), want .github/ci.yml", changed, err)
	}
}

// A staged rename is listed by its new path only: the old one no longer
// exists and is not a change of the task's.
func TestTaskScopeCheckListsARenameByItsNewPath(t *testing.T) {
	dir := gitRepository(t, map[string]string{"old name.go": "package x\n"})
	cmd := exec.Command("git", "mv", "old name.go", "new name.go")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git mv: %v %s", err, out)
	}
	writeFile(t, dir, "added.go", "package x\n")
	paths, git, err := (&CeremonyDriver{Checks: gitDirChecks{dir: dir}}).gitChanged(context.Background())
	if err != nil || !git || strings.Join(paths, "|") != "added.go|new name.go" {
		t.Fatalf("paths %q git=%v err=%v", paths, git, err)
	}
}

// Without -z git quotes a non-ASCII path with octal escapes ("caf\303\251.md")
// and the parser only trimmed the quotes, so a legitimate scope file read as
// a change outside the scope and green was refused for good.
func TestTaskScopeCheckReadsNonASCIIPaths(t *testing.T) {
	dir := gitRepository(t, nil)
	writeFile(t, dir, "café.md", "x")
	writeFile(t, dir, "docs/ñandú (copia).md", "x")
	d := &CeremonyDriver{Checks: gitDirChecks{dir: dir}, Files: gitDirFiles{dir: dir}}
	task := &domain.TaskRun{Scope: []string{"café.md", "docs/ñandú (copia).md"}, Git: true, Start: map[string]string{}}
	changed, err := d.changedByTask(context.Background(), task)
	if err != nil || strings.Join(changed, "|") != "café.md|docs/ñandú (copia).md" {
		t.Fatalf("changed %q err=%v, want the two scope files by name", changed, err)
	}
}

// truncatedStatus answers git status like the runtime when its output cap
// cut the answer.
type truncatedStatus struct{}

func (truncatedStatus) Run(context.Context, domain.CheckCommand) (CheckResult, error) {
	return CheckResult{Ran: true, Stdout: "?? a.go\x00?? b", Truncated: true}, nil
}

func TestTaskScopeCheckRefusesACutGitStatus(t *testing.T) {
	if _, _, err := (&CeremonyDriver{Checks: truncatedStatus{}}).gitChanged(context.Background()); err == nil || !strings.Contains(err.Error(), "scope cannot be checked") {
		t.Fatalf("a cut git status must fail loudly: %v", err)
	}
}
