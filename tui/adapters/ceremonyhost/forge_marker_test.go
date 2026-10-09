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
	want := "git " + strings.Join(stageArgs, " ")
	if !strings.Contains(strings.Join(checks.ran, "\n"), want) {
		t.Fatalf("missing %q in\n%s", want, strings.Join(checks.ran, "\n"))
	}
	// The same arguments, without a shell, in a real repository.
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return string(out)
	}
	run("init", "-q")
	for name, content := range map[string]string{application.RepairMarker: "{}\n", "fix.go": "package x\n", "sub/" + application.RepairMarker: "{}\n"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run(stageArgs...)
	if staged := strings.Fields(run("diff", "--cached", "--name-only")); strings.Join(staged, ",") != "fix.go,sub/"+application.RepairMarker {
		t.Fatalf("staged %v, want the fix and no root marker", staged)
	}
}
