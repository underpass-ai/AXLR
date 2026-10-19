package application

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// v2Answer is one scripted answer of v2Judge: the family it expects to be
// asked, and the verdict or error it returns.
type v2Answer struct {
	family  string
	verdict JudgementVerdict
	err     error
}

// v2Judge answers axlr-v2 questions in sequence, each with a full valid
// distribution unless the test scripts otherwise, and fails the test when
// a question of another family, or one too many, arrives.
type v2Judge struct {
	t         *testing.T
	answers   []v2Answer
	questions []JudgementQuestion
}

func (j *v2Judge) Judge(_ context.Context, q JudgementQuestion) (JudgementVerdict, error) {
	j.t.Helper()
	j.questions = append(j.questions, q)
	if len(j.questions) > len(j.answers) {
		j.t.Fatalf("question %d not scripted: %.80s", len(j.questions), q.Question)
	}
	answer := j.answers[len(j.questions)-1]
	if q.Question != judgeV2Questions[answer.family] {
		j.t.Fatalf("question %d is not %s: %.80s", len(j.questions), answer.family, q.Question)
	}
	return answer.verdict, answer.err
}

const v2Model = "jev-1.13.0"

func v2Yes(p float64) v2Answer {
	return v2Answer{verdict: JudgementVerdict{Model: v2Model, Yes: &p}}
}

// v2Pick answers a choice among options with p on choice and the rest
// spread evenly over the other options.
func v2Pick(options []string, choice string, p float64) JudgementVerdict {
	probabilities := map[string]float64{}
	rest := (1 - p) / float64(len(options)-1)
	for _, option := range options {
		probabilities[option] = rest
	}
	probabilities[choice] = p
	confidence := 0.8
	return JudgementVerdict{Model: v2Model, Choice: choice, Probabilities: probabilities, Confidence: &confidence}
}

func gateAnswer(p float64) v2Answer {
	a := v2Yes(p)
	a.family = JudgeFamilyGate
	return a
}

func choiceAnswer(family string, options []string, choice string, p float64) v2Answer {
	return v2Answer{family: family, verdict: v2Pick(options, choice, p)}
}

func v2Decisions() JudgeDecisions {
	return JudgeDecisions{Policy: JudgePolicyAXLRv2, Model: v2Model}
}

// dossierSession records a lab dossier as a session turn: the prompt, one
// assistant message with the dossier's calls, their results and the
// answer. A dossier with a call without result cannot be recorded.
func dossierSession(t *testing.T, d JudgeDossier) (domain.Session, bool) {
	t.Helper()
	s := turnSession(t)
	if err := s.BeginTurn(root.Text(d.Prompt), turnTools()); err != nil {
		t.Fatal(err)
	}
	var calls []root.ToolCall
	for i, tool := range d.Tools {
		if tool.Result == nil {
			return s, false
		}
		raw, err := json.Marshal(tool.Arguments)
		if err != nil {
			t.Fatal(err)
		}
		args, err := root.NewJSONObject(raw)
		if err != nil {
			return s, false
		}
		calls = append(calls, root.ToolCall{ID: root.ToolCallID(itoa(i + 1)), Name: root.ToolName(tool.Name), Arguments: args})
	}
	if len(calls) > 0 {
		if err := s.CompleteAssistantWithin(assistant("", calls...), 100); err != nil {
			t.Fatal(err)
		}
		for i, tool := range d.Tools {
			if err := s.RecordToolOutcome(calls[i].ID, domain.DecisionAutoApprove, domain.ToolOutcome{Content: root.Text(*tool.Result)}); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := s.CompleteAssistant(assistant(root.Text(d.Answer))); err != nil {
		t.Fatal(err)
	}
	return s, true
}

// The questions the console sends from a session are the oracle's: the
// final check's from the transcript, the gate's and the sentence's from
// what settle asks. A sentence question is compared only where the
// console's own sentence extraction finds the dossier's candidates; the
// others are counted as extraction differences, which the lab does not
// measure (it supplies the candidates).
func TestJudgeV2SessionQuestionsMatchTheOracle(t *testing.T) {
	paths := []string{"testdata/jev_v2/synthetic.jsonl"}
	if private := os.Getenv("AXLR_JEV_V2_ORACLE"); private != "" {
		paths = append(paths, private)
	}
	for _, path := range paths {
		counts := map[string]int{}
		for _, row := range readOracle(t, path) {
			if row.Question == nil || row.GoDiverges != "" || (row.Family != JudgeFamilyFinal && row.Family != JudgeFamilyGate && row.Family != JudgeFamilySentence) {
				continue
			}
			s, recorded := dossierSession(t, row.Dossier)
			if !recorded {
				counts["not recordable"]++
				continue
			}
			want := *row.Question
			var got JudgementQuestion
			switch row.Family {
			case JudgeFamilyFinal:
				messages := s.Messages()
				judge := &Judge{Decisions: v2Decisions()}
				q, err := judge.finalQuestion(messages, personRequest(messages))
				if err != nil {
					t.Fatalf("%s: %v", row.ID, err)
				}
				got = q
			default:
				record := BuildTurnRecord(s, 0, 1, "project:fixture", nil)
				if !record.HasResults() {
					counts["settle guard: no results"]++
					continue
				}
				sentences := settleSentences(row.Dossier.Answer)
				answers := []v2Answer{gateAnswer(0.99)}
				if len(sentences) > 0 {
					options := append(optionIDs("s", len(sentences)), "none")
					answers = append(answers, choiceAnswer(JudgeFamilySentence, options, "none", 0.9))
				}
				capture := &v2Judge{t: t, answers: answers}
				c := SessionCeremony{Judge: capture, Journal: &journalStub{}, Decisions: v2Decisions()}
				c.settle(context.Background(), s, domain.SessionInstance{}, record, "step:fixture")
				if row.Family == JudgeFamilyGate {
					got = capture.questions[0]
					break
				}
				if !reflect.DeepEqual(sentences, row.Dossier.Sentences) {
					counts["sentence extraction differs"]++
					continue
				}
				got = capture.questions[1]
			}
			if got.State != want.State || got.Question != want.Question || len(got.Options) != len(want.Options) || (len(got.Options) > 0 && !reflect.DeepEqual(got.Options, want.Options)) {
				t.Errorf("%s: session question differs:\ngo     %s\noracle %s", row.ID, got.State, want.State)
			}
			counts[row.Family]++
		}
		t.Logf("%s: identical from a session %v", path, counts)
	}
}

func optionIDs(prefix string, n int) []string {
	var out []string
	for i := 1; i <= n; i++ {
		out = append(out, prefix+itoa(i))
	}
	return out
}

// The evidence is the turn's: its own prompt and answer, every call with
// its whole result (an absent one apart from an empty one), and nothing of
// the next turn.
func TestJudgeTurnEvidenceReadsTheWholeTurnAndOnlyIt(t *testing.T) {
	long := `{"protocol_version":1,"tool":"local_exec","status":"completed","output":{"exit_code":2,"stdout":"` + strings.Repeat("ok ", 300) + `FAIL: la migración no terminó","stderr":""},"error":null}`
	args := func(text string) root.JSONValue { o, _ := root.NewJSONObject([]byte(text)); return o }
	messages := []root.Message{
		{Role: root.RoleUser, Content: "Migra la base y comprueba\n\n[AXLR · recall] Memory of project:x"},
		{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{
			{ID: "a", Name: "local_exec", Arguments: args(`{"program":"migrate","retries":3}`)},
			{ID: "b", Name: "local_read", Arguments: args(`{"path":"MIGRATION.md"}`)},
			{ID: "c", Name: "local_write", Arguments: args(`{"path":"x"}`)},
			{ID: "d", Name: "local_edit", Arguments: args(`{"path":"y"}`)},
		}},
		{Role: root.RoleTool, ToolCallID: "a", Content: root.Text(long)},
		{Role: root.RoleTool, ToolCallID: "b", Content: "Antes de migrar, haz copia de seguridad."},
		{Role: root.RoleTool, ToolCallID: "d", Content: ""},
		{Role: root.RoleAssistant, Content: "La migración terminó."},
		{Role: root.RoleUser, Content: "[AXLR · memory] ¿Algo que recordar?"},
		{Role: root.RoleAssistant, Content: "Nada que recordar."},
		{Role: root.RoleUser, Content: "Ahora otra cosa"},
		{Role: root.RoleAssistant, ToolCalls: []root.ToolCall{{ID: "e", Name: "local_exec", Arguments: args(`{}`)}}},
		{Role: root.RoleTool, ToolCallID: "c", Content: "result of another turn"},
	}
	evidence, err := JudgeTurnEvidenceOf(messages, 0, false)
	if err != nil {
		t.Fatal(err)
	}
	if evidence.PersonRequest != "Migra la base y comprueba" || evidence.Answer != "La migración terminó." || len(evidence.Tools) != 4 {
		t.Fatalf("%+v", evidence)
	}
	if evidence.Tools[0].Result == nil || *evidence.Tools[0].Result != long || evidence.Tools[0].Arguments.(map[string]any)["retries"] != json.Number("3") {
		t.Fatalf("whole result or arguments lost: %+v", evidence.Tools[0])
	}
	if evidence.Tools[2].Result != nil || evidence.Tools[3].Result == nil || *evidence.Tools[3].Result != "" {
		t.Fatalf("absent and empty results: %+v %+v", evidence.Tools[2], evidence.Tools[3])
	}
	final, err := JudgeTurnEvidenceOf(messages[:8], 0, true)
	if err != nil || final.Answer != "Nada que recordar." {
		t.Fatalf("final reads the last message: %+v %v", final, err)
	}
	if _, err := JudgeTurnEvidenceOf(messages[:2], 0, true); err == nil {
		t.Fatal("a turn ending on calls has no final answer")
	}
	if _, err := JudgeTurnEvidenceOf(messages, 1, false); err == nil {
		t.Fatal("an assistant message is no prompt")
	}
	next, err := JudgeTurnEvidenceOf(messages, 8, false)
	if err != nil || next.PersonRequest != "Ahora otra cosa" || len(next.Tools) != 1 || next.Tools[0].Result != nil {
		t.Fatalf("next turn %+v %v", next, err)
	}
	compiled, err := CompileJudgeV2(JudgeFamilyFinal, evidence.Dossier())
	if err != nil {
		t.Fatal(err)
	}
	for _, kept := range []string{"FAIL: la migración no terminó", `\"exit_code\":2`, "Antes de migrar", `"result":null`, `"result":""`} {
		if !strings.Contains(compiled.Question.State, kept) {
			t.Errorf("state lost %q", kept)
		}
	}
}

var errTransport = errors.New("TypeSafe transport failure")
