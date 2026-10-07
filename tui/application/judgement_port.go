package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	root "github.com/underpass-ai/AXLR/domain"
)

// JudgementPort asks an external judgement model (TypeSafe Jev) one typed
// question about a state it is given. It never sees the session: only the
// state and the question sent.
type JudgementPort interface {
	Judge(context.Context, JudgementQuestion) (JudgementVerdict, error)
}

// JudgementQuestion is a yes/no question when Options is empty, otherwise a
// choice among Options.
type JudgementQuestion struct {
	State    string
	Question string
	Options  []string
}

// Limits keep one judgement inside one TypeSafe request (24k estimated
// tokens at 1.5 bytes per token) with room for the question.
const (
	MaxJudgementStateBytes    = 24 << 10
	MaxJudgementQuestionBytes = 2000
	MaxJudgementOptions       = 16
	MaxJudgementOptionBytes   = 200
)

func (q JudgementQuestion) Validate() error {
	if strings.TrimSpace(q.State) == "" || len(q.State) > MaxJudgementStateBytes || !utf8.ValidString(q.State) {
		return fmt.Errorf("state must be 1 to %d bytes of text", MaxJudgementStateBytes)
	}
	if strings.TrimSpace(q.Question) == "" || len(q.Question) > MaxJudgementQuestionBytes || !utf8.ValidString(q.Question) {
		return fmt.Errorf("question must be 1 to %d bytes of text", MaxJudgementQuestionBytes)
	}
	if len(q.Options) == 1 || len(q.Options) > MaxJudgementOptions {
		return fmt.Errorf("options must be omitted for a yes/no question or list 2 to %d choices", MaxJudgementOptions)
	}
	seen := map[string]bool{}
	for _, option := range q.Options {
		if strings.TrimSpace(option) == "" || len(option) > MaxJudgementOptionBytes || !utf8.ValidString(option) {
			return fmt.Errorf("each option must be 1 to %d bytes of text", MaxJudgementOptionBytes)
		}
		if seen[option] {
			return errors.New("options must be distinct")
		}
		seen[option] = true
	}
	return nil
}

// JudgementVerdict is Jev's answer: Yes is the probability of yes for a
// yes/no question; Choice, Probabilities and Confidence answer a choice.
type JudgementVerdict struct {
	Model         string             `json:"model"`
	Yes           *float64           `json:"yes,omitempty"`
	Choice        string             `json:"choice,omitempty"`
	Probabilities map[string]float64 `json:"probabilities,omitempty"`
	Confidence    *float64           `json:"confidence,omitempty"`
}

// Judge enables TypeSafe Jev in a console. Tool offers axlr_judge to the
// model; FinalCheck asks Jev, when the model ends a request with a final
// answer, whether that answer completes it, and below Threshold returns the
// verdict to the model once.
type Judge struct {
	Port       JudgementPort
	Tool       bool
	FinalCheck bool
	Threshold  float64
}

func (j *Judge) offersTool() bool  { return j != nil && j.Port != nil && j.Tool }
func (j *Judge) checksFinal() bool { return j != nil && j.Port != nil && j.FinalCheck }

func (j *Judge) toolPort() JudgementPort {
	if !j.offersTool() {
		return nil
	}
	return j.Port
}

func withoutTool(tools []root.ToolDefinition, name root.ToolName) []root.ToolDefinition {
	out := tools[:0:0]
	for _, tool := range tools {
		if tool.Name != name {
			out = append(out, tool)
		}
	}
	return out
}

// hostJudge is axlr_judge: it validates the model's question, asks Jev and
// returns the verdict as the tool result.
func hostJudge(ctx context.Context, port JudgementPort, arguments root.JSONValue) (JudgementVerdict, error) {
	var input struct {
		State    string   `json:"state"`
		Question string   `json:"question"`
		Options  []string `json:"options"`
	}
	decoder := json.NewDecoder(bytes.NewReader(arguments.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return JudgementVerdict{}, errors.New("axlr_judge takes state, question and optional options")
	}
	question := JudgementQuestion{State: input.State, Question: input.Question, Options: input.Options}
	if err := question.Validate(); err != nil {
		return JudgementVerdict{}, err
	}
	verdict, err := port.Judge(ctx, question)
	if err != nil {
		return JudgementVerdict{}, fmt.Errorf("Jev did not answer: %w", err)
	}
	return verdict, nil
}
