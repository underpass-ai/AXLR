package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// memoryTools is a session snapshot with local tools, the host tools and
// KMP's wake and write tools.
func memoryTools(t *testing.T) []domain.AvailableTool {
	t.Helper()
	return append(repairTools(t), hostPlugin(t, "kmp_write_memory", "kmp", "kmp_write_memory"))
}

func memoryRequest(t *testing.T, calls ...root.ToolCall) []root.Message {
	t.Helper()
	request := []root.Message{{Role: root.RoleUser, Content: "fix the parser"}}
	if len(calls) > 0 {
		request = append(request, root.Message{Role: root.RoleAssistant, ToolCalls: calls})
		for _, call := range calls {
			request = append(request, root.Message{Role: root.RoleTool, ToolCallID: call.ID, Content: `{"status":"completed","output":{}}`})
		}
	}
	return append(request, root.Message{Role: root.RoleAssistant, Content: "done"})
}

// withResult replaces the result of one call in a request.
func withResult(request []root.Message, id root.ToolCallID, content root.Text) []root.Message {
	out := append([]root.Message(nil), request...)
	for i := range out {
		if out[i].Role == root.RoleTool && out[i].ToolCallID == id {
			out[i].Content = content
		}
	}
	return out
}

func toolCall(t *testing.T, id string, name root.ToolName, arguments string) root.ToolCall {
	t.Helper()
	return root.ToolCall{ID: root.ToolCallID(id), Name: name, Arguments: mustObject(t, arguments)}
}

func TestMemoryReminderNeedsDurableWorkAndNoWrite(t *testing.T) {
	edit := toolCall(t, "e1", "local_edit", `{}`)
	reads, commands := []root.ToolCall{}, []root.ToolCall{}
	for i := 1; i <= 8; i++ {
		reads = append(reads, toolCall(t, fmt.Sprintf("r%d", i), "local_read", `{}`))
	}
	for i := 1; i <= 5; i++ {
		commands = append(commands, toolCall(t, fmt.Sprintf("x%d", i), "local_exec", `{}`))
	}
	// Searching and listing change nothing, like reading (9 Oct 2026).
	searches := []root.ToolCall{}
	for i := 1; i <= 4; i++ {
		searches = append(searches, toolCall(t, fmt.Sprintf("s%d", i), "local_search", `{"pattern":"x"}`), toolCall(t, fmt.Sprintf("l%d", i), "local_list", `{}`))
	}
	for _, tc := range []struct {
		name    string
		request []root.Message
		want    bool
	}{
		{"a file change", memoryRequest(t, edit), true},
		// A denied, failed or cancelled write changed nothing (10 Oct 2026).
		{"a denied edit", withResult(memoryRequest(t, edit), "e1", "tool call denied by user"), false},
		{"a failed edit", withResult(memoryRequest(t, edit), "e1", `{"status":"failed","error":{"code":"invalid_arguments","message":"x"}}`), false},
		{"a cancelled edit", withResult(memoryRequest(t, edit), "e1", "tool call cancelled"), false},
		{"an edit without a result", []root.Message{{Role: root.RoleUser, Content: "fix the parser"}, {Role: root.RoleAssistant, ToolCalls: []root.ToolCall{edit}}, {Role: root.RoleAssistant, Content: "done"}}, false},
		{"a denied and a completed write", withResult(memoryRequest(t, edit, toolCall(t, "w2", "local_write", `{}`)), "e1", "tool call denied by user"), true},
		{"five commands", memoryRequest(t, commands...), true},
		// One command that ran can settle an outcome (decision F2); one
		// the person denied, or that never started, settled nothing.
		{"one command and reads", memoryRequest(t, reads[0], reads[1], commands[0]), true},
		{"a denied command", withResult(memoryRequest(t, commands[0]), "x1", "tool call denied by user"), false},
		{"a refused command", withResult(memoryRequest(t, commands[0]), "x1", "give why: one sentence saying what this call is for"), false},
		{"a command without a result", []root.Message{{Role: root.RoleUser, Content: "run the tests"}, {Role: root.RoleAssistant, ToolCalls: []root.ToolCall{commands[0]}}, {Role: root.RoleAssistant, Content: "done"}}, false},
		{"a forged tool run", memoryRequest(t, toolCall(t, "f1", HostRunToolName, `{"name":"t"}`)), true},
		// A refused axlr_run_tool never ran its program: its result is an
		// error envelope (hostFailure), with no status (docs/console.md).
		{"a forged tool changed since it was forged", withResult(memoryRequest(t, toolCall(t, "f1", HostRunToolName, `{"name":"t"}`)), "f1", `{"error":"forged tool \"t\" changed since it was forged (count.sh); forge it again"}`), false},
		{"an unknown forged tool", withResult(memoryRequest(t, toolCall(t, "f1", HostRunToolName, `{"name":"missing"}`)), "f1", `{"error":"unknown forged tool \"missing\""}`), false},
		{"a forged tool run with invalid arguments", withResult(memoryRequest(t, toolCall(t, "f1", HostRunToolName, `{}`)), "f1", `{"error":"axlr_run_tool arguments must be name, arguments and why"}`), false},
		{"eight reads", memoryRequest(t, reads...), false},
		{"searches and listings", memoryRequest(t, searches...), false},
		{"searches and a recall", memoryRequest(t, searches[0], searches[1], toolCall(t, "q1", HostRecallName, `{}`)), false},
		{"a question", memoryRequest(t, reads[:2]...), false},
		{"session startup", memoryRequest(t, toolCall(t, "b1", HostSessionName, `{}`), toolCall(t, "b2", HostSkillName, `{}`), toolCall(t, "b3", HostToolsName, `{}`), toolCall(t, "b4", HostCallToolName, `{"name":"kmp_guide","arguments":{}}`), toolCall(t, "b5", HostCallToolName, `{"name":"kmp_wake","arguments":{}}`), toolCall(t, "b6", "kmp_wake", `{}`)), false},
		{"bridged write", memoryRequest(t, edit, toolCall(t, "w1", HostCallToolName, `{"name":"kmp_write_memory","arguments":{}}`)), false},
		{"direct write", memoryRequest(t, edit, toolCall(t, "w1", "kmp_write_memory", `{}`)), false},
		{"a ceremony", memoryRequest(t, edit, toolCall(t, "s1", HostStepDoneName, `{}`)), false},
		{"already reminded", append(memoryRequest(t, edit), root.Message{Role: root.RoleUser, Content: memoryReminder}, root.Message{Role: root.RoleAssistant, Content: "nothing durable"}), false},
	} {
		if got := needsMemoryReminder(tc.request, memoryWriteSnapshot()); got != tc.want {
			t.Fatalf("%s: got %v", tc.name, got)
		}
	}
	messages := []root.Message{{Role: root.RoleUser, Content: "first"}, {Role: root.RoleAssistant, Content: "a"}, {Role: root.RoleUser, Content: "second\n\n[AXLR] Self-repair r merged."}, {Role: root.RoleUser, Content: "[AXLR · Jev] doubt"}}
	if personRequest(messages) != 2 {
		t.Fatal("a console note is not the person's request; a note appended to a prompt is")
	}
}

func TestModelIsToldToRecordOnlyWhereItRecordsMemory(t *testing.T) {
	s := turnSession(t)
	if err := s.BeginTurn("fix the parser", memoryTools(t)); err != nil {
		t.Fatal(err)
	}
	text := string(modelHostGuidance(&s, engineParagraphs{}).Content)
	if !strings.Contains(text, "Record what the work settles") || !strings.Contains(text, "kmp_write_memory = kmp_write_memory") {
		t.Fatalf("normal mode guidance: %s", text)
	}
	if err := s.CompleteAssistant(assistant("done")); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMode(domain.ModeIncident); err != nil {
		t.Fatal(err)
	}
	if modelRecordsMemory(s) || strings.Contains(string(modelHostGuidance(&s, engineParagraphs{}).Content), "Record what the work settles") {
		t.Fatal("incident mode refuses the model's memory writes; it must not be told to record")
	}
	noKMP := turnSession(t)
	if err := noKMP.BeginTurn("fix the parser", repairTools(t)); err != nil {
		t.Fatal(err)
	}
	if modelRecordsMemory(noKMP) {
		t.Fatal("without kmp_write_memory there is nothing to remind")
	}
}

func TestMemoryReminderAndJevEachActOncePerRequest(t *testing.T) {
	stub := &judgeStub{verdict: JudgementVerdict{Yes: yes(0.2)}}
	var lasts []string
	u := agentWith(&Judge{Port: stub, FinalCheck: true}, func(_ context.Context, r root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		last := string(r.Messages[len(r.Messages)-1].Content)
		lasts = append(lasts, last)
		switch len(lasts) {
		case 1:
			return assistant("", root.ToolCall{ID: "e1", Name: "local_edit", Arguments: mustObject(t, `{}`)}), nil
		case 2:
			return assistant("Done: the parser splits on whitespace."), nil
		case 3:
			if !strings.HasPrefix(last, jevFinalPrefix) {
				t.Fatalf("third request should carry Jev's doubt: %q", last)
			}
			return assistant("Done, verified."), nil
		case 4:
			if !strings.HasPrefix(last, MemoryReminderPrefix) {
				t.Fatalf("fourth request should carry the memory reminder: %q", last)
			}
			// The reminder names the memory write the snapshot holds.
			if strings.Contains(last, "%!") || !strings.Contains(last, "axlr_remember") && !strings.Contains(last, "kmp_write_memory through axlr_call_tool") {
				t.Fatalf("the reminder names no memory write: %q", last)
			}
			return assistant("Nothing durable to record: a one-line fix."), nil
		}
		t.Fatalf("an extra model turn after %q", last)
		return root.CompletionResult{}, nil
	})
	u.Approval = approvesAll{}
	u.Tools = toolFunc(func(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error) {
		return domain.ToolOutcome{Content: `{"status":"completed","output":{}}`}, nil
	})
	s := turnSession(t)
	if err := s.BeginTurn("fix the parser", memoryTools(t)); err != nil {
		t.Fatal(err)
	}
	if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil {
		t.Fatal(err)
	}
	if len(lasts) != 4 || len(stub.questions) != 1 || s.Status() != domain.StatusComplete {
		t.Fatalf("turns=%d judged=%d status=%s", len(lasts), len(stub.questions), s.Status())
	}
	if !strings.Contains(stub.questions[0].State, "fix the parser") {
		t.Fatalf("Jev judged another request: %s", stub.questions[0].State)
	}
}

// Jev settles a turn at its record: where the turn ends in none (MADE
// unavailable, so no instance), the reminder still asks the model.
func TestMemoryReminderStandsWhereNoRecordSettlesTheTurn(t *testing.T) {
	durableTurn := func() domain.Session {
		s := turnSession(t)
		if err := s.BeginTurn("fix the parser", memoryTools(t)); err != nil {
			t.Fatal(err)
		}
		return s
	}
	finish := func(s *domain.Session) {
		edit := toolCall(t, "e1", "local_edit", `{}`)
		if err := s.CompleteAssistant(assistant("", edit)); err != nil {
			t.Fatal(err)
		}
		if err := s.RecordToolOutcome("e1", domain.DecisionAutoApprove, domain.ToolOutcome{Content: `{"status":"completed","output":{}}`}); err != nil {
			t.Fatal(err)
		}
		if err := s.CompleteAssistant(assistant("done")); err != nil {
			t.Fatal(err)
		}
	}
	engine := &fakeEngine{ready: errors.New("connection refused")}
	c, _, _, _ := sessionCeremonyFor(t, engine)
	c.Judge = &scriptedJudge{}
	s := durableTurn()
	c.BeginTurn(context.Background(), &s)
	finish(&s)
	if reminded, err := remindMemory(context.Background(), &s, ContinueTurnUseCase{Store: &memoryStore{}, Session: c}, ignoreEvent); !reminded || err != nil {
		t.Fatalf("no record, no reminder: %v %v", reminded, err)
	}
	engine.ready = nil
	s = durableTurn()
	c.BeginTurn(context.Background(), &s)
	finish(&s)
	if reminded, err := remindMemory(context.Background(), &s, ContinueTurnUseCase{Store: &memoryStore{}, Session: c}, ignoreEvent); reminded || err != nil {
		t.Fatalf("reminded where Jev settles: %v %v", reminded, err)
	}
}

// A command that only reads is no work to record: on 20 November 2026 a
// local model was reminded after cat big.txt, spent a request and stored
// "big.txt contains ñáé…" in KMP. Calls from wrx80 sessions that day.
func TestACommandThatOnlyReadsIsNoWorkToRecord(t *testing.T) {
	for _, tc := range []struct {
		arguments string
		readOnly  bool
	}{
		{`{"program":"cat","args":["big.txt"]}`, true},
		{`{"program":"sqlmap","args":["--version"]}`, true},
		{`{"program":".venv-sqlmap/bin/sqlmap","args":["--version"]}`, true},
		{`{"program":"git","args":["-C","x","status","--short"]}`, true},
		{`{"program":"find","args":[".","-name","*.go"]}`, true},
		{`{"program":"sh","args":["-c","cat /etc/os-release; echo ---; for p in apt-get pacman go pip; do command -v $p >/dev/null 2>&1 && echo \"$p: $(command -v $p)\"; done"]}`, true},
		{`{"program":"sh","args":["-c","for d in /usr/bin /etc; do if [ -w $d ]; then echo \"$d ok\"; else echo \"$d no\"; fi; done"]}`, true},
		{`{"program":"sh","args":["-c","nmap -V | head -1; grep -c ERROR logs/a.log 2>/dev/null"]}`, true},
		{`{"program":"sh","args":["-c","pacman -Ss nmap | head -3"]}`, false},
		{`{"program":"pip3","args":["install","--user","sqlmap"]}`, false},
		{`{"program":"sh","args":["-c","mkdir -p .bin && GOBIN=$PWD/.bin go install example.com/x@latest"]}`, false},
		{`{"program":"sh","args":["-c","echo hi > notes.txt"]}`, false},
		{`{"program":"sh","args":["-c","printf 'x' > /tmp/check.txt; grep -c x /tmp/check.txt"]}`, true},
		{`{"program":"find","args":[".","-name","*.tmp","-delete"]}`, false},
		{`{"program":"git","args":["commit","-m","x"]}`, false},
		{`{"program":"command","args":["rm","-rf","build"]}`, false},
		{`{"program":"env","args":["FOO=1","make"]}`, false},
		{`{"program":"go","args":["test","./..."]}`, false},
		{`{"program":"python3","args":["-m","unittest"]}`, false},
		// A word "version" that is not the probe, and options that write
		// (21 November 2026).
		{`{"program":"npm","args":["version"]}`, true},
		{`{"program":"kubectl","args":["version","--client"]}`, true},
		{`{"program":"npm","args":["version","patch"]}`, false},
		{`{"program":"rm","args":["-rf","version"]}`, false},
		{`{"program":"git","args":["tag","version"]}`, false},
		{`{"program":"git","args":["--version"]}`, true},
		{`{"program":"sh","args":["-c","git tag version"]}`, false},
		{`{"program":"go","args":["env","GOPATH"]}`, true},
		{`{"program":"go","args":["env","-w","GOFLAGS=-mod=mod"]}`, false},
		{`{"program":"sort","args":["-uo","out.txt","in.txt"]}`, false},
		{`{"program":"find","args":[".","-fls","list.txt"]}`, false},
		{`{"program":"uniq","args":["in.txt","out.txt"]}`, false},
		{`{"program":"uniq","args":["-c","in.txt"]}`, true},
		{`{"program":"tree","args":["-o","tree.txt"]}`, false},
		{`{"program":"git","args":["diff","--output=patch.diff"]}`, false},
		// What a substitution or a case arm runs counts too, and only a
		// plain /tmp path or a whole device name is harmless.
		{`{"program":"sh","args":["-c","echo $(rm -rf build)"]}`, false},
		{`{"program":"sh","args":["-c","echo ` + "`rm -rf build`" + `"]}`, false},
		{`{"program":"sh","args":["-c","echo \"go: $(command -v go)\""]}`, true},
		{`{"program":"sh","args":["-c","ls; case x in x) rm -rf b;; esac"]}`, false},
		{`{"program":"sh","args":["-c","case $1 in -v) echo v;; esac"]}`, true},
		{`{"program":"sh","args":["-c","echo x > /dev/ttyUSB0"]}`, false},
		{`{"program":"sh","args":["-c","echo x > /tmp/../home/me/notes"]}`, false},
		{`{"program":"sh","args":["-c","printf x > /tmp/a;rm -rf b"]}`, false},
		// System probes read too; ip, systemctl and journalctl are judged
		// by their verb (wrx80, 21 November 2026: uptime -p cost a turn).
		{`{"program":"uptime","args":["-p"]}`, true},
		{`{"program":"sh","args":["-c","lsblk; pgrep -af llama; ss -ltnp | grep 18083"]}`, true},
		{`{"program":"hexdump","args":["-C","a.bin"]}`, true},
		{`{"program":"ip","args":["-br","addr","show"]}`, true},
		{`{"program":"ip","args":["link","set","eth0","down"]}`, false},
		{`{"program":"ip","args":["netns","exec","n1","rm","-rf","x"]}`, false},
		{`{"program":"systemctl","args":["--user","status","axlr-node"]}`, true},
		{`{"program":"systemctl","args":["--user","restart","axlr-node"]}`, false},
		{`{"program":"systemctl","args":["--failed"]}`, true},
		{`{"program":"journalctl","args":["--user","-u","axlr-node","-n","50"]}`, true},
		{`{"program":"journalctl","args":["--vacuum-time=1d"]}`, false},
		{`{"program":"yarn","args":["version","--patch"]}`, false},
		{`{"program":"yarn","args":["version","--new-version=2.0.0"]}`, false},
		{`{"program":"ss","args":["-K","dst","10.0.0.1"]}`, false},
		{`{"program":"ss","args":["-ltnp"]}`, true},
		{`{"program":"sh","args":["-c","diff <(rm -rf build) /dev/null"]}`, false},
		{`{"program":"sh","args":["-c","diff <(sort a.txt) <(sort b.txt)"]}`, true},
	} {
		if got := readOnlyExec(mustObject(t, tc.arguments)); got != tc.readOnly {
			t.Errorf("%s: read only = %v, want %v", tc.arguments, got, tc.readOnly)
		}
	}
	cat := toolCall(t, "c1", "local_exec", `{"program":"cat","args":["big.txt"]}`)
	install := toolCall(t, "p1", "local_exec", `{"program":"pip3","args":["install","sqlmap"]}`)
	if needsMemoryReminder(memoryRequest(t, cat), memoryWriteSnapshot()) {
		t.Fatal("a turn that only ran cat was reminded")
	}
	if !needsMemoryReminder(memoryRequest(t, cat, install), memoryWriteSnapshot()) {
		t.Fatal("a turn that installed something was not reminded")
	}
}
