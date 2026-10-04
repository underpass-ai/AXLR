package ceremonyhost

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// scriptedChecks answers each command from a script keyed by its first words;
// unknown commands succeed silently. It records what ran.
type scriptedChecks struct {
	answers map[string]application.CheckResult
	ran     []string
}

func (s *scriptedChecks) Run(_ context.Context, c domain.CheckCommand) (application.CheckResult, error) {
	line := c.Program + " " + strings.Join(c.Args, " ")
	s.ran = append(s.ran, line)
	for prefix, answer := range s.answers {
		if strings.HasPrefix(line, prefix) {
			return answer, nil
		}
	}
	return application.CheckResult{Ran: true}, nil
}

func ok(output string) application.CheckResult {
	return application.CheckResult{Ran: true, Output: output}
}

func TestForgeProposeOpensThePullRequestAsTheConsole(t *testing.T) {
	checks := &scriptedChecks{answers: map[string]application.CheckResult{
		"git diff --cached --quiet": {Ran: true, ExitCode: 1},
		"git rev-parse HEAD":        ok("abc123\n"),
		"gh pr create":              ok("Creating pull request\nhttps://github.com/o/r/pull/12\n"),
	}}
	pr, err := Forge{Checks: checks}.Propose(context.Background(), application.RepairProposal{Repository: "o/r", Base: "main", Branch: "repair/x", Title: "Repair: x", Body: "body", Trailer: "Repaired-by: AXLR"})
	if err != nil {
		t.Fatal(err)
	}
	if pr.Number != 12 || pr.URL != "https://github.com/o/r/pull/12" || pr.HeadSHA != "abc123" {
		t.Fatalf("pr %+v", pr)
	}
	joined := strings.Join(checks.ran, "\n")
	for _, want := range []string{"git checkout -B repair/x", "git add -A", "user.name=" + CommitAuthor, "user.email=" + CommitEmail, "Repaired-by: AXLR", "git push -q -u origin repair/x", "gh pr create --repo o/r --base main --head repair/x --title Repair: x --body body"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in\n%s", want, joined)
		}
	}
}

func TestForgeProposePushesToTheOpenPullRequestOnALaterRound(t *testing.T) {
	checks := &scriptedChecks{answers: map[string]application.CheckResult{
		"git diff --cached --quiet": {Ran: true, ExitCode: 1},
		"git rev-parse HEAD":        ok("def456"),
		"gh pr view 12":             ok("https://github.com/o/r/pull/12"),
	}}
	pr, err := Forge{Checks: checks}.Propose(context.Background(), application.RepairProposal{Repository: "o/r", Base: "main", Branch: "repair/x", Number: 12, Title: "t", Body: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if pr.Number != 12 || pr.HeadSHA != "def456" || pr.URL != "https://github.com/o/r/pull/12" {
		t.Fatalf("pr %+v", pr)
	}
	joined := strings.Join(checks.ran, "\n")
	if strings.Contains(joined, "checkout -B") || strings.Contains(joined, "gh pr create") {
		t.Fatalf("second round must not branch or create: %s", joined)
	}
}

func TestForgeProposeRefusesAnEmptyRepairAndReportsCommandFailures(t *testing.T) {
	checks := &scriptedChecks{answers: map[string]application.CheckResult{"git diff --cached --quiet": {Ran: true, ExitCode: 0}}}
	if _, err := (Forge{Checks: checks}).Propose(context.Background(), application.RepairProposal{Repository: "o/r", Base: "main", Branch: "b"}); err == nil || !strings.Contains(err.Error(), "changed no file") {
		t.Fatalf("empty repair: %v", err)
	}
	checks = &scriptedChecks{answers: map[string]application.CheckResult{
		"git diff --cached --quiet": {Ran: true, ExitCode: 1},
		"git push":                  {Ran: true, ExitCode: 128, Output: "remote: Permission denied"},
	}}
	if _, err := (Forge{Checks: checks}).Propose(context.Background(), application.RepairProposal{Repository: "o/r", Base: "main", Branch: "b"}); err == nil || !strings.Contains(err.Error(), "Permission denied") {
		t.Fatalf("push failure: %v", err)
	}
	if _, err := (Forge{Checks: checks}).Propose(context.Background(), application.RepairProposal{}); err == nil {
		t.Fatal("a proposal without repository must be refused")
	}
	checks = &scriptedChecks{answers: map[string]application.CheckResult{
		"git diff --cached --quiet": {Ran: true, ExitCode: 1},
		"gh pr create":              ok("no url here"),
	}}
	if _, err := (Forge{Checks: checks}).Propose(context.Background(), application.RepairProposal{Repository: "o/r", Base: "main", Branch: "b"}); err == nil || !strings.Contains(err.Error(), "no pull request URL") {
		t.Fatalf("missing url: %v", err)
	}
}

func TestForgeStatusCountsCheckRunsAndCommitStatuses(t *testing.T) {
	view := `{"state":"OPEN","mergeStateStatus":"BLOCKED","headRefOid":"h1","statusCheckRollup":[
	 {"name":"go","status":"COMPLETED","conclusion":"FAILURE","detailsUrl":"https://ci/1"},
	 {"name":"brand","status":"COMPLETED","conclusion":"SUCCESS"},
	 {"name":"chart","status":"IN_PROGRESS","conclusion":""},
	 {"context":"CodeQL","state":"SUCCESS","targetUrl":"https://cq"},
	 {"context":"preflight","state":"PENDING"},
	 {"name":"skipped","status":"COMPLETED","conclusion":"SKIPPED"}]}`
	checks := &scriptedChecks{answers: map[string]application.CheckResult{"gh pr view 9": ok(view)}}
	status, err := Forge{Checks: checks}.Status(context.Background(), "o/r", 9)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "OPEN" || status.MergeState != "BLOCKED" || status.HeadSHA != "h1" || status.Pending != 2 || status.Passed != 3 || len(status.Failed) != 1 || status.Failed[0] != "go: failure https://ci/1" {
		t.Fatalf("status %+v", status)
	}
	checks = &scriptedChecks{answers: map[string]application.CheckResult{"gh pr view 9": ok("not json")}}
	if _, err := (Forge{Checks: checks}).Status(context.Background(), "o/r", 9); err == nil {
		t.Fatal("unreadable view must fail")
	}
	checks = &scriptedChecks{answers: map[string]application.CheckResult{"gh pr view 9": {Ran: false, ExitCode: -1, Output: "gh: command not found"}}}
	if _, err := (Forge{Checks: checks}).Status(context.Background(), "o/r", 9); err == nil || !strings.Contains(err.Error(), "command not found") {
		t.Fatalf("missing gh: %v", err)
	}
}

func TestForgeMergeAndUpdateBranch(t *testing.T) {
	checks := &scriptedChecks{answers: map[string]application.CheckResult{"gh pr view 9": ok("m3rge\n")}}
	sha, err := Forge{Checks: checks}.Merge(context.Background(), "o/r", 9)
	if err != nil || sha != "m3rge" {
		t.Fatalf("merge %q %v", sha, err)
	}
	if !strings.Contains(strings.Join(checks.ran, "\n"), "gh pr merge 9 --repo o/r --squash --delete-branch") {
		t.Fatal(checks.ran)
	}
	if err := (Forge{Checks: checks}).UpdateBranch(context.Background(), "o/r", 9); err != nil || !strings.Contains(strings.Join(checks.ran, "\n"), "gh pr update-branch 9 --repo o/r") {
		t.Fatalf("update-branch %v %v", err, checks.ran)
	}
	checks = &scriptedChecks{answers: map[string]application.CheckResult{"gh pr merge": {Ran: true, ExitCode: 1, Output: "Pull request is not mergeable"}}}
	if _, err := (Forge{Checks: checks}).Merge(context.Background(), "o/r", 9); err == nil || !strings.Contains(err.Error(), "not mergeable") {
		t.Fatalf("merge failure: %v", err)
	}
}

type failingChecks struct{}

func (failingChecks) Run(context.Context, domain.CheckCommand) (application.CheckResult, error) {
	return application.CheckResult{}, errors.New("runtime closed")
}

func TestForgePropagatesRuntimeErrors(t *testing.T) {
	if _, err := (Forge{Checks: failingChecks{}}).Status(context.Background(), "o/r", 1); err == nil || !strings.Contains(err.Error(), "runtime closed") {
		t.Fatalf("got %v", err)
	}
}

func TestWakeFocusedPassesTheIntentAndReturnsRefs(t *testing.T) {
	var request map[string]any
	m := Memory{Tools: memoryToolsFunc(func(_ context.Context, id domain.ToolIdentity, args root.JSONValue) (domain.ToolOutcome, error) {
		if err := json.Unmarshal(args.Bytes(), &request); err != nil {
			t.Fatal(err)
		}
		packet := map[string]any{"wake": map[string]any{
			"current_state": []any{"project:x:entry:decision:a (decision): one", "project:x:entry:decision:a (decision): repeated", "no ref here"},
			"open_loops":    []any{"project:x:entry:observation:b (observation): two"},
		}}
		encoded, _ := json.Marshal(packet)
		return domain.ToolOutcome{Content: root.Text(encoded)}, nil
	})}
	text, refs, err := m.WakeFocused(context.Background(), "project:x", "the failure")
	if err != nil {
		t.Fatal(err)
	}
	if request["intent"] != "the failure" || request["about"] != "project:x" {
		t.Fatalf("request %v", request)
	}
	if len(refs) != 2 || refs[0] != "project:x:entry:decision:a" || refs[1] != "project:x:entry:observation:b" {
		t.Fatalf("refs %v", refs)
	}
	if !strings.Contains(text, "one") || !strings.Contains(text, "two") {
		t.Fatalf("text %q", text)
	}
}

func TestRecordLinkedSendsLinksAndReturnsTheStoredRef(t *testing.T) {
	var request map[string]any
	m := Memory{Tools: memoryToolsFunc(func(_ context.Context, _ domain.ToolIdentity, args root.JSONValue) (domain.ToolOutcome, error) {
		if err := json.Unmarshal(args.Bytes(), &request); err != nil {
			t.Fatal(err)
		}
		return domain.ToolOutcome{Content: `{"accepted":true,"local_refs":{"c1":"project:x:entry:error_path:c1-deadbeef"}}`}, nil
	})}
	ref, err := m.RecordLinked(context.Background(), "project:x", map[string][]string{"ceremony": {"axlr_repair"}}, application.MemoryRecord{ID: "c1", Kind: "error_path", Summary: "s", Evidence: "e", Links: []application.MemoryLink{{Ref: "project:x:entry:decision:a", Rel: "restates", Why: "same", Evidence: "probe"}}})
	if err != nil || ref != "project:x:entry:error_path:c1-deadbeef" {
		t.Fatalf("ref %q err %v", ref, err)
	}
	memories := request["memories"].([]any)
	memory := memories[0].(map[string]any)
	if _, sent := memory["summary_en"]; sent {
		t.Fatal("summary_en must not be sent")
	}
	links := memory["connect_to"].([]any)
	link := links[0].(map[string]any)
	if link["ref"] != "project:x:entry:decision:a" || link["rel"] != "restates" || link["why"] != "same" || link["evidence"] != "probe" || link["confidence"] != "medium" {
		t.Fatalf("link %v", link)
	}
}
