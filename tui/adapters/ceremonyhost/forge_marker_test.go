package ceremonyhost

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/application"
)

// Propose stages the clone's changes without the repair marker even when
// .git/info/exclude does not list it: the marker holds the brief, the about
// and the origin session, which do not belong in the pull request.
func TestForgeProposeNeverStagesTheRepairMarker(t *testing.T) {
	checks := &scriptedChecks{answers: map[string]application.CheckResult{
		"git diff --cached --quiet": {Ran: true, ExitCode: 1},
		"git rev-parse HEAD":        ok("abc123\n"),
		"gh pr create":              ok("https://github.com/o/r/pull/12\n"),
	}}
	if _, err := (Forge{Checks: checks}).Propose(context.Background(), application.RepairProposal{Repository: "o/r", Base: "main", Branch: "b", Title: "t", Body: "b"}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"git " + strings.Join(stageArgs, " "), "git " + strings.Join(unstageArgs, " ")} {
		if !strings.Contains(strings.Join(checks.ran, "\n"), want) {
			t.Fatalf("missing %q in\n%s", want, strings.Join(checks.ran, "\n"))
		}
	}
	// The same arguments, without a shell, in a real repository, with the
	// marker in .git/info/exclude as the preparer writes it and without.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	for _, excluded := range []bool{true, false} {
		dir := t.TempDir()
		run := func(args ...string) string {
			t.Helper()
			cmd := exec.Command("git", args...)
			cmd.Dir = dir
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("excluded=%v: git %v: %v %s", excluded, args, err, out)
			}
			return string(out)
		}
		run("init", "-q")
		if excluded {
			if err := os.WriteFile(filepath.Join(dir, ".git", "info", "exclude"), []byte(application.RepairMarker+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		for name, content := range map[string]string{application.RepairMarker: "{}\n", "fix.go": "package x\n", "sub/" + application.RepairMarker: "{}\n"} {
			if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		run(stageArgs...)
		run(unstageArgs...)
		want := "fix.go,sub/" + application.RepairMarker
		if excluded {
			want = "fix.go" // info/exclude matches the marker at any depth
		}
		if staged := strings.Fields(run("diff", "--cached", "--name-only")); strings.Join(staged, ",") != want {
			t.Fatalf("excluded=%v: staged %v, want %s", excluded, staged, want)
		}
	}
}
