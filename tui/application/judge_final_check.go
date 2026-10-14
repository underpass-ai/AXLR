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
const jevFinalPrefix = domain.JevNotePrefix

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
// console turn that shows the verdict to the model, with the tool calls the
// request has left (domain.JevNotePrefix); with none left there is nothing
// the follow-up could do, and Jev is not asked. Outside the normal flow
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
	if personDecided(messages[start:]) {
		return false, nil
	}
	left := u.turnLimit() - s.RequestCallsUsed()
	if left <= 0 {
		return false, nil
	}
	state, err := finalCheckState(messages[start:])
	if err != nil {
		return false, nil
	}
	started := time.Now()
	verdict, err := u.Judge.Port.Judge(ctx, JudgementQuestion{State: state, Question: finalCheckQuestion})
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
	note := finalCheckNote(*verdict.Yes, left)
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

// finalCheckQuestion is what Jev is asked. A call the person denied or
// cancelled is their decision: on 14 October 2026 the person denied
// `go vet ./...` ("no hace falta vet, ya basta"), Jev's doubt counted it as
// missing work and the model ran the denied command again.
const finalCheckQuestion = "Does `answer` complete what `request` asks? Judge only from the request, the answer and the tool activity given; an answer that stops at a plan, asks to continue or reports an unfinished step does not complete it. A tool call denied or cancelled by the person is the person's decision, not missing work."

// finalCheckNote is the follow-up turn's message. The model's next answer
// reaches the person as a reply, so the note asks for one written to the
// person that never mentions the check: on 14 October 2026 glm-5.3-flash
// answered the note itself ("La respuesta está completa; no falta nada por
// hacer") and the person read it as a second reply.
func finalCheckNote(yes float64, left int) root.Text {
	calls := fmt.Sprintf("%d tool calls", left)
	if left == 1 {
		calls = "1 tool call"
	}
	return root.Text(fmt.Sprintf("%s An independent check that saw only the person's request, your last answer and the tool activity estimates a %.0f%% probability that your answer completes the request. If something the person asked is still missing, do it now (this request has %s left) and then tell the person what changed. Never repeat a call the person denied or cancelled: that was their decision. If nothing is missing, tell the person so in one sentence. Write to the person, not to this check, and do not mention it.", jevFinalPrefix, yes*100, calls))
}

// personDecided reports a request in which the person denied or cancelled
// a tool call: the person already decided what the request does, so its
// final answer is not checked. On 14 October 2026, after a denial, Jev
// doubted the answer and the follow-up spent 60k tokens on bookkeeping
// (a title, two axlr_remember calls, local_list .kernel) for a 3-line
// file.
func personDecided(turn []root.Message) bool {
	for _, message := range turn {
		if message.Role != root.RoleTool {
			continue
		}
		if outcome, _ := classifyOutcome(string(message.Content)); outcome == turnDenied || outcome == turnCancelled {
			return true
		}
	}
	return false
}

// finalCheckState is what Jev sees: the request as the person wrote it,
// the final answer and one line per tool call of the request, each
// bounded, with how it ended. The console's notes appended to the prompt
// (the recall note, a ceremony step) are left out: on 14 October 2026 Jev
// judged "Di hola en una palabra." with the [AXLR · recall] memory of
// other work glued on, text the person never wrote.
func finalCheckState(turn []root.Message) (string, error) {
	var tools []string
	results := map[root.ToolCallID]string{}
	for _, message := range turn {
		if message.Role == root.RoleTool {
			results[message.ToolCallID] = string(message.Content)
		}
	}
	for _, message := range turn {
		for _, call := range message.ToolCalls {
			if len(tools) == finalToolLines {
				break
			}
			tools = append(tools, fmt.Sprintf("%s %s: %s", call.Name, cut(string(call.Arguments.Bytes()), 160), finalCallStatus(results[call.ID])))
		}
	}
	state, err := json.Marshal(map[string]any{
		"request":       cut(personTyped(string(turn[0].Content)), finalRequestBytes),
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

// finalCallStatus says how a call ended, as Jev reads it: ok, error, denied
// or cancelled by the person, or not run over the allowance; a command's
// non-zero exit is named, since the call itself ran.
func finalCallStatus(result string) string {
	outcome, _ := classifyOutcome(result)
	switch outcome {
	case turnCompleted:
		if code, _ := execResult(result); code != nil && *code != 0 {
			return fmt.Sprintf("ok, exit %d", *code)
		}
		return "ok"
	case turnDenied:
		return "denied by the person"
	case turnCancelled:
		return "cancelled by the person"
	case turnOverBudget:
		return "not run (over the tool-call allowance)"
	case turnUnknown:
		return "no result"
	}
	return "error"
}
