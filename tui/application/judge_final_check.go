package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// jevFinalPrefix opens the console's message when Jev doubts a final answer.
// A request that already received it is not checked again, so the check
// costs at most one extra model turn per request.
const jevFinalPrefix = "[AXLR · Jev]"

// DefaultJudgeThreshold is the probability of completion below which the
// final check returns Jev's verdict to the model.
const DefaultJudgeThreshold = 0.5

const (
	finalRequestBytes = 6 << 10
	finalAnswerBytes  = 12 << 10
	finalToolLines    = 40
)

// checkFinalAnswer asks Jev, once per request, whether the model's final
// answer completes what the person asked. Below the threshold it starts one
// console turn that shows the verdict to the model. Outside the normal flow
// (a live ceremony, a pending decision) and on any Jev failure it does
// nothing: the check can only add a turn, never block one.
func checkFinalAnswer(ctx context.Context, s *domain.Session, u ContinueTurnUseCase, emit func(Event) error) (bool, error) {
	if !u.Judge.checksFinal() || s.Status() != domain.StatusComplete {
		return false, nil
	}
	if _, live := s.Ceremony(); live {
		return false, nil
	}
	messages := s.Messages()
	if len(messages) == 0 || messages[len(messages)-1].Role != root.RoleAssistant || len(messages[len(messages)-1].ToolCalls) > 0 {
		return false, nil
	}
	// The request is the person's last message with everything after it,
	// console messages included, so a later console turn (a memory reminder,
	// a cancelled ceremony) neither repeats the check nor stands in for the
	// person's request.
	start := personRequest(messages)
	if start < 0 {
		return false, nil
	}
	for _, message := range messages[start+1:] {
		if message.Role == root.RoleUser && strings.HasPrefix(string(message.Content), jevFinalPrefix) {
			return false, nil
		}
	}
	state, err := finalCheckState(messages[start:])
	if err != nil {
		return false, nil
	}
	started := time.Now()
	verdict, err := u.Judge.Port.Judge(ctx, JudgementQuestion{State: state, Question: "Does `answer` complete what `request` asks? Judge only from the request, the answer and the tool activity given; an answer that stops at a plan, asks to continue or reports an unfinished step does not complete it."})
	if u.Diagnostics != nil {
		class := DiagnosticErrorNone
		if err != nil {
			class = DiagnosticErrorProvider
		}
		_ = u.Diagnostics.Record(DiagnosticEvent{Stage: DiagnosticJudgement, ElapsedMilliseconds: time.Since(started).Milliseconds(), ErrorClass: class})
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	threshold := u.Judge.Threshold
	if threshold <= 0 || threshold >= 1 {
		threshold = DefaultJudgeThreshold
	}
	if err != nil || verdict.Yes == nil || *verdict.Yes >= threshold {
		return false, nil
	}
	note := root.Text(fmt.Sprintf("%s Jev, an independent judge that saw only the request, your answer and the tool activity, estimates a %.0f%% probability that your answer completes the request. Check what is missing with your tools and finish the task, or say briefly why the answer is complete. The console asks this once per request.", jevFinalPrefix, *verdict.Yes*100))
	next := *s
	if err := next.BeginTurn(note, next.ToolSnapshot()); err != nil {
		return false, err
	}
	if err := u.Store.Save(ctx, next); err != nil {
		return false, err
	}
	*s = next
	return true, emitSession(s, emit)
}

// finalCheckState is what Jev sees: the request, the final answer and one
// line per tool call of the request, each bounded.
func finalCheckState(turn []root.Message) (string, error) {
	var tools []string
	results := map[root.ToolCallID]bool{}
	for _, message := range turn {
		if message.Role == root.RoleTool {
			results[message.ToolCallID] = !strings.Contains(string(message.Content), `"error"`)
		}
	}
	for _, message := range turn {
		for _, call := range message.ToolCalls {
			if len(tools) == finalToolLines {
				break
			}
			status := "error"
			if results[call.ID] {
				status = "ok"
			}
			tools = append(tools, fmt.Sprintf("%s %s: %s", call.Name, cut(string(call.Arguments.Bytes()), 160), status))
		}
	}
	state, err := json.Marshal(map[string]any{
		"request":       cut(string(turn[0].Content), finalRequestBytes),
		"answer":        cut(string(turn[len(turn)-1].Content), finalAnswerBytes),
		"tool_activity": tools,
	})
	return string(state), err
}

// cut keeps at most limit bytes of text on a rune boundary.
func cut(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	for limit > 0 && !utf8.RuneStart(text[limit]) {
		limit--
	}
	return text[:limit] + "…"
}
