package application

import (
	"context"
	"errors"
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
