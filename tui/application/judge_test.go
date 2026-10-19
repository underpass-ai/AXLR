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

type judgeStub struct {
	questions []JudgementQuestion
	verdict   JudgementVerdict
	err       error
}

func (j *judgeStub) Judge(_ context.Context, q JudgementQuestion) (JudgementVerdict, error) {
	j.questions = append(j.questions, q)
	return j.verdict, j.err
}

func yes(p float64) *float64 { return &p }

func judgeCall(id root.ToolCallID, args string) root.ToolCall {
	value, _ := root.NewJSONObject([]byte(args))
	return root.ToolCall{ID: id, Name: HostJudgeName, Arguments: value}
}

func agentWith(judge *Judge, models streamFunc) AgentTurnUseCase {
	return AgentTurnUseCase{Continue: ContinueTurnUseCase{Store: &memoryStore{}, Models: models, Judge: judge}}
}

func TestJudgeToolIsOfferedOnlyWhenEnabled(t *testing.T) {
	stub := &judgeStub{}
	run := func(judge *Judge, s *domain.Session) bool {
		offered := false
		u := ContinueTurnUseCase{Store: &memoryStore{}, Judge: judge, Models: streamFunc(func(_ context.Context, r root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
			offered = requestHasTool(r, HostJudgeName)
			return assistant("ok"), nil
		})}
		if err := s.BeginTurn("go", s.ToolSnapshot()); err != nil {
			t.Fatal(err)
		}
		if err := u.Execute(context.Background(), s, ignoreEvent); err != nil {
			t.Fatal(err)
		}
		return offered
	}
	s := turnSession(t)
	if run(nil, &s) || run(&Judge{Port: stub, FinalCheck: true}, &s) {
		t.Fatal("axlr_judge offered without the tool switch")
	}
	if !run(&Judge{Port: stub, Tool: true}, &s) {
		t.Fatal("axlr_judge missing with the tool switch on")
	}
	// The session now keeps axlr_judge in its snapshot; turning Jev off must
	// still hide it.
	if run(nil, &s) {
		t.Fatal("axlr_judge offered after Jev was turned off")
	}
}

func TestJudgeToolAsksJevAndReturnsTheVerdict(t *testing.T) {
	stub := &judgeStub{verdict: JudgementVerdict{Model: "jev-1.13.0", Choice: "b", Probabilities: map[string]float64{"a": 0.1, "b": 0.9}, Confidence: yes(0.8)}}
	calls := 0
	var result string
	u := agentWith(&Judge{Port: stub, Tool: true}, func(_ context.Context, r root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		calls++
		if calls == 1 {
			return assistant("", judgeCall("j1", `{"state":"two fixes","question":"Which fix?","options":["a","b"]}`)), nil
		}
		result = string(r.Messages[len(r.Messages)-1].Content)
		return assistant("done"), nil
	})
	s := turnSession(t)
	if err := s.BeginTurn("fix it", turnTools()); err != nil {
		t.Fatal(err)
	}
	if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil {
		t.Fatal(err)
	}
	if len(stub.questions) != 1 || stub.questions[0].Options[1] != "b" || !strings.Contains(result, `"choice":"b"`) || calls != 2 {
		t.Fatalf("questions=%+v result=%s calls=%d", stub.questions, result, calls)
	}
}

func TestJudgeFinalCheckReturnsADoubtedAnswerOnce(t *testing.T) {
	stub := &judgeStub{verdict: JudgementVerdict{Yes: yes(0.2)}}
	calls := 0
	u := agentWith(&Judge{Port: stub, FinalCheck: true}, func(_ context.Context, r root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		calls++
		if calls == 1 {
			return assistant("", call("r1", "read")), nil
		}
		if calls == 2 {
			return assistant("I would start by reading the file."), nil
		}
		last := string(r.Messages[len(r.Messages)-1].Content)
		if !strings.HasPrefix(last, jevFinalPrefix) || !strings.Contains(last, "20%") {
			t.Fatalf("console note = %q", last)
		}
		return assistant("Done: the file is fixed."), nil
	})
	u.Approval = approvesAll{}
	u.Tools = toolFunc(func(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error) {
		return domain.ToolOutcome{Content: "contents"}, nil
	})
	s := turnSession(t)
	if err := s.BeginTurn("fix the file", turnTools()); err != nil {
		t.Fatal(err)
	}
	if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil {
		t.Fatal(err)
	}
	if calls != 3 || len(stub.questions) != 1 || s.Status() != domain.StatusComplete {
		t.Fatalf("calls=%d judged=%d status=%s", calls, len(stub.questions), s.Status())
	}
	state := stub.questions[0].State
	if !strings.Contains(state, "fix the file") || !strings.Contains(state, "I would start") || !strings.Contains(state, "read {}: ok") {
		t.Fatalf("state = %s", state)
	}
}

func TestJudgeFinalCheckLetsConfidentOrFailedJudgementsPass(t *testing.T) {
	for name, stub := range map[string]*judgeStub{
		"confident": {verdict: JudgementVerdict{Yes: yes(0.9)}},
		"failure":   {err: errors.New("offline")},
	} {
		calls := 0
		u := agentWith(&Judge{Port: stub, FinalCheck: true}, func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
			calls++
			return assistant("done"), nil
		})
		s := turnSession(t)
		if err := s.BeginTurn("go", turnTools()); err != nil {
			t.Fatal(err)
		}
		if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil || calls != 1 || len(stub.questions) != 1 {
			t.Fatalf("%s: calls=%d judged=%d err=%v", name, calls, len(stub.questions), err)
		}
	}
}

// Jev reads how each call ended from its envelope: `"error":null` is a
// success, a denial or cancellation is the person's. On 14 October 2026
// every local result read "error" because it holds `"error":null`, and Jev
// doubted three of four correct answers.
func TestJudgeFinalCheckReadsHowEachCallEnded(t *testing.T) {
	for result, want := range map[string]string{
		`{"protocol_version":1,"tool":"read","status":"completed","error":null,"output":{"content":"x"}}`: "ok",
		`{"status":"completed","error":null,"output":{"exit_code":1,"stdout":"FAIL"}}`:                    "ok, exit 1",
		`{"status":"failed","error":{"code":"not_found","message":"x"}}`:                                  "error",
		"tool call denied by user: no hace falta vet, ya basta":                                           "denied by the person",
		cancelledByPerson:                             "cancelled by the person",
		"tool call cancelled":                         "cancelled by the person",
		"tool execution failed; effect unknown: boom": "error",
	} {
		if got := finalCallStatus(result); got != want {
			t.Fatalf("%q: %q, want %q", result, got, want)
		}
	}
	state, err := finalCheckState([]root.Message{
		{Role: root.RoleUser, Content: "vet it"},
		{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{call("v", "exec")}},
		{Role: root.RoleTool, ToolCallID: "v", Content: "tool call denied by user: ya basta"},
		{Role: root.RoleAssistant, Content: "Not run: you denied it."},
	})
	if err != nil || !strings.Contains(state, "exec {}: denied by the person") {
		t.Fatalf("%s %v", state, err)
	}
	// The question and the note say a denied or cancelled call is not
	// missing work; the note asks for an answer to the person, never to
	// the check, one sentence when nothing is missing.
	if !strings.Contains(finalCheckQuestion, "denied or cancelled by the person is the person's decision, not missing work") {
		t.Fatal(finalCheckQuestion)
	}
	note := string(finalCheckNote(0.27, 3))
	for _, want := range []string{"27%", "3 tool calls left", "Never repeat a call the person denied or cancelled", "in one sentence", "can only come from the person", "instead of guessing or searching outside the workspace", "Write to the person, not to this check, and do not mention it"} {
		if !strings.Contains(note, want) {
			t.Fatalf("note lacks %q: %s", want, note)
		}
	}
	if strings.Contains(note, "judge") || strings.Contains(note, "say briefly why the answer is complete") {
		t.Fatalf("the note invites an answer to the judge: %s", note)
	}
}

// The follow-up gets the allowance the request left; with none left Jev is
// not asked (14 October 2026: turn_tool_calls 8, the request used 8 and the
// follow-up ran 8 more).
func TestJudgeFinalCheckIsSkippedWithNoCallsLeft(t *testing.T) {
	for _, limit := range []int{8, 9} {
		stub := &judgeStub{verdict: JudgementVerdict{Yes: yes(0.2)}}
		calls := 0
		var note string
		u := agentWith(&Judge{Port: stub, FinalCheck: true}, func(_ context.Context, r root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
			calls++
			switch calls {
			case 1:
				var batch []root.ToolCall
				for i := 0; i < 8; i++ {
					batch = append(batch, call(root.ToolCallID(fmt.Sprintf("r%d", i)), "read"))
				}
				return assistant("", batch...), nil
			case 2:
				return assistant("Read them all."), nil
			}
			note = string(r.Messages[len(r.Messages)-1].Content)
			return assistant("Nothing is missing."), nil
		})
		u.Continue.TurnToolCalls = limit
		u.Approval = approvesAll{}
		u.Tools = toolFunc(func(context.Context, domain.ToolIdentity, root.JSONValue) (domain.ToolOutcome, error) {
			return domain.ToolOutcome{Content: `{"status":"completed","error":null,"output":{}}`}, nil
		})
		s := turnSession(t)
		if err := s.BeginTurn("read the files", turnTools()); err != nil {
			t.Fatal(err)
		}
		if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil {
			t.Fatal(err)
		}
		if limit == 8 && (calls != 2 || len(stub.questions) != 0) {
			t.Fatalf("with no calls left: model calls %d, Jev asked %d times", calls, len(stub.questions))
		}
		if limit == 9 {
			if left, _ := s.CallsLeft(limit); calls != 3 || len(stub.questions) != 1 || !strings.Contains(note, "1 tool call left") || left != 1 {
				t.Fatalf("with one call left: model calls %d, Jev %d, left %d, note %q", calls, len(stub.questions), left, note)
			}
		}
	}
}

// On 14 October 2026 Jev was sent "Di hola en una palabra." with the
// console's [AXLR · recall] note glued on, and after a denied call its
// doubt sent the model into 60k tokens of bookkeeping. Jev now reads only
// what the person wrote, and a request in which the person denied or
// cancelled a call is not checked: the person already decided.
func TestJudgeFinalCheckReadsThePersonAndSkipsTheirDecisions(t *testing.T) {
	state, err := finalCheckState([]root.Message{
		{Role: root.RoleUser, Content: "Di hola en una palabra.\n\n" + RecallNotePrefix + " Memory of project:ws:\n- calc.Max returns the largest"},
		{Role: root.RoleAssistant, Content: "Hola"},
	})
	if err != nil || !strings.Contains(state, `"request":"Di hola en una palabra."`) || strings.Contains(state, "calc.Max") {
		t.Fatalf("%s %v", state, err)
	}
	for result, checked := range map[string]bool{"tool call denied by user: no": false, cancelledByPerson: false, "contents": true} {
		stub := &judgeStub{verdict: JudgementVerdict{Yes: yes(0.9)}}
		s := turnSession(t)
		if err := s.BeginTurn("lee calc.go", turnTools()); err != nil {
			t.Fatal(err)
		}
		if err := s.CompleteAssistant(assistant("", call("r1", "read"))); err != nil {
			t.Fatal(err)
		}
		decision := domain.DecisionApprove
		if !checked {
			decision = domain.DecisionDeny
		}
		if err := s.RecordToolOutcome("r1", decision, domain.ToolOutcome{Content: root.Text(result), IsError: !checked}); err != nil {
			t.Fatal(err)
		}
		if err := s.CompleteAssistant(assistant("No lo leí.")); err != nil {
			t.Fatal(err)
		}
		u := ContinueTurnUseCase{Store: &memoryStore{}, Judge: &Judge{Port: stub, FinalCheck: true}}
		if _, err := checkFinalAnswer(context.Background(), &s, u, ignoreEvent); err != nil || (len(stub.questions) == 1) != checked {
			t.Fatalf("%q: judged %d, want checked=%v (%v)", result, len(stub.questions), checked, err)
		}
	}
}
