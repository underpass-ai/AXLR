package application

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	root "github.com/underpass-ai/AXLR/domain"
)

// JudgeToolEvidence is one tool call of a turn as axlr-v2 shows it to Jev:
// its name, its arguments as JSON (numbers as written) and the whole
// content of its result, envelope and exit code included. Result is nil
// when the call has no result, which is not an empty one, and nothing is
// inferred from the call alone.
type JudgeToolEvidence struct {
	Name      string  `json:"name"`
	Arguments any     `json:"arguments"`
	Result    *string `json:"result"`
}

// UnmarshalJSON reads a lab dossier's tool, keeping the arguments'
// numbers as written.
func (t *JudgeToolEvidence) UnmarshalJSON(data []byte) error {
	var wire struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
		Result    *string         `json:"result"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	arguments, err := judgeArguments(wire.Arguments)
	if err != nil {
		return err
	}
	*t = JudgeToolEvidence{Name: wire.Name, Arguments: arguments, Result: wire.Result}
	return nil
}

func (t JudgeToolEvidence) state() map[string]any {
	var result any
	if t.Result != nil {
		result = redactResult(*t.Result)
	}
	return map[string]any{"name": t.Name, "arguments": t.Arguments, "result": result}
}

// redactResult redacts a JSON result value by value: a result is compact
// JSON text, and Redact's KEY=value runs to the next space, so over the
// text it would swallow the fields after a secret, the exit code
// included. The lab redacts the text whole; the two differ only on a
// result that holds a secret. Any other result is left to the state's
// redaction.
func redactResult(result string) string {
	if !secretPattern.MatchString(result) {
		return result
	}
	value, err := judgeArguments([]byte(result))
	if err != nil {
		return result
	}
	switch value.(type) {
	case map[string]any, []any:
	default:
		return result
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if encoder.Encode(redactValue(value)) != nil {
		return result
	}
	return unescapeLineSeparators(bytes.TrimSuffix(buffer.Bytes(), []byte("\n")))
}

// JudgeTurnEvidence is one turn as axlr-v2 judges it: what the person
// typed, the answer that reached them and every tool call of the turn,
// reads included. It is the judge's ephemeral evidence: never stored in
// LastRecord, a log or memory, and not cut to the record's excerpts.
type JudgeTurnEvidence struct {
	PersonRequest string
	Answer        string
	Tools         []JudgeToolEvidence
}

// Dossier is the evidence as a decision's dossier.
func (e JudgeTurnEvidence) Dossier() JudgeDossier {
	return JudgeDossier{Prompt: e.PersonRequest, Answer: e.Answer, Tools: e.Tools}
}

// JudgeTurnEvidenceOf reads the turn that starts at the person's prompt at
// index, bounded as BuildTurnRecord bounds it: console messages ([AXLR …])
// belong to the turn, the person's next message ends it. Results are
// joined to their calls by ToolCallID inside the turn only. The answer is
// the one the turn record keeps (the model's answer before a console
// message's bookkeeping) or, with final, the transcript's last message,
// which the final check judges.
func JudgeTurnEvidenceOf(messages []root.Message, index int, final bool) (JudgeTurnEvidence, error) {
	if index < 0 || index >= len(messages) || messages[index].Role != root.RoleUser {
		return JudgeTurnEvidence{}, errors.New("no person prompt at the turn's index")
	}
	end, answerAt := turnSpan(messages, index)
	evidence := JudgeTurnEvidence{PersonRequest: PersonTyped(string(messages[index].Content))}
	switch {
	case final:
		last := messages[end-1]
		if last.Role != root.RoleAssistant || len(last.ToolCalls) > 0 {
			return JudgeTurnEvidence{}, errors.New("the turn does not end on a final answer")
		}
		evidence.Answer = string(last.Content)
	case answerAt >= 0:
		evidence.Answer = string(messages[answerAt].Content)
	}
	results := map[root.ToolCallID]string{}
	for _, message := range messages[index:end] {
		if message.Role == root.RoleTool {
			results[message.ToolCallID] = string(message.Content)
		}
	}
	for _, message := range messages[index+1 : end] {
		for _, call := range message.ToolCalls {
			arguments, err := judgeArguments(call.Arguments.Bytes())
			if err != nil {
				return JudgeTurnEvidence{}, fmt.Errorf("arguments of %s: %w", call.Name, err)
			}
			tool := JudgeToolEvidence{Name: string(call.Name), Arguments: arguments}
			if result, ok := results[call.ID]; ok {
				tool.Result = &result
			}
			evidence.Tools = append(evidence.Tools, tool)
		}
	}
	return evidence, nil
}

// judgeArguments decodes a call's arguments as a JSON value whose numbers
// keep their text, so the state carries an object, not a quoted string.
func judgeArguments(raw []byte) (any, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errors.New("trailing data after the arguments")
	}
	return value, nil
}
