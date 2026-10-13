package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// The context cap (decision D10 of the lifecycle integration design): a
// request's context (guidance, tool schemas, messages) never exceeds the
// cap in tokens, counted with one tokenizer for every model. Nothing is
// dropped or compacted in place, so the provider's prompt cache keeps its
// prefix; when the next request would exceed the cap, the turn is
// interrupted, the session's instance closed, and the person's prompt
// continues in a successor session whose first prompt recalls the
// predecessor's memory.

// messageTokenOverhead is what a message costs beyond its text: the role
// and the framing, as OpenAI counts them.
const messageTokenOverhead = 4

// RequestTokens counts a request: the guidance, every tool definition and
// every message, each text counted once by the counter's memo.
func RequestTokens(counter TokenCounterPort, guidance root.Message, tools []root.ToolDefinition, messages []root.Message) int {
	if counter == nil {
		return 0
	}
	total := messageTokens(counter, guidance)
	for _, tool := range tools {
		encoded, _ := json.Marshal(map[string]any{"name": tool.Name, "description": tool.Description, "parameters": json.RawMessage(tool.Parameters.Bytes())})
		total += counter.Count(string(encoded)) + messageTokenOverhead
	}
	for _, message := range messages {
		total += messageTokens(counter, message)
	}
	return total
}

func messageTokens(counter TokenCounterPort, message root.Message) int {
	total := messageTokenOverhead + counter.Count(string(message.Content))
	for _, call := range message.ToolCalls {
		total += messageTokenOverhead + counter.Count(string(call.Name)) + counter.Count(string(call.Arguments.Bytes()))
	}
	return total
}

// cappedBudget keeps a budget's per-result bounds and removes its cuts:
// with the cap on, the projection drops no turn and writes no checkpoint.
func cappedBudget(budget domain.ContextBudget) domain.ContextBudget {
	ceiling := domain.DefaultContextBudget()
	uncut, err := domain.NewContextBudget(ceiling.MaximumBytes(), ceiling.LowWaterBytes(), budget.ToolResultBytes(), budget.CheckpointBytes())
	if err != nil {
		return budget
	}
	return uncut
}

// capsContext reports whether the cap applies to the session's requests:
// a counter and a cap exist, and no compact step runs (its ledger
// projection bounds itself).
func (u ContinueTurnUseCase) capsContext(s domain.Session) (int, bool) {
	if u.Tokens == nil || u.ContextCap == nil {
		return 0, false
	}
	if _, _, compact := compactRun(s); compact {
		return 0, false
	}
	cap := u.ContextCap(sessionModel(s))
	return cap, cap > 0
}

// continueInSuccessor interrupts the turn, closes the session's instance
// and opens the successor the person's prompt continues in.
func (u ContinueTurnUseCase) continueInSuccessor(ctx context.Context, session *domain.Session, tokens, cap int) error {
	if u.Successor == nil {
		return fmt.Errorf("the request would reach %d tokens, past the cap of %d, and no successor session can be opened", tokens, cap)
	}
	if firstRequest(session.Messages()) {
		// The prompt alone exceeds the cap: a successor would take the
		// same prompt and exceed it again, forever.
		return fmt.Errorf("the prompt alone would reach %d tokens, past the cap of %d (prompt_tokens in settings): shorten it or raise the cap", tokens, cap)
	}
	next := *session
	if next.Status() == domain.StatusStreaming {
		if err := next.InterruptDraft(""); err != nil {
			return err
		}
	}
	prompt := lastPersonPrompt(next.Messages())
	if err := u.Store.Save(context.WithoutCancel(ctx), next); err != nil {
		return err
	}
	*session = next
	if u.Session != nil {
		_ = u.Session.Close(context.WithoutCancel(ctx), session, fmt.Sprintf("continued at %d tokens", tokens))
	}
	successor, err := u.Successor.Continue(context.WithoutCancel(ctx), *session)
	if err != nil {
		return fmt.Errorf("the request would reach %d tokens, past the cap of %d, and the successor session could not be opened: %w", tokens, cap, err)
	}
	return &ContextCapError{Tokens: tokens, Cap: cap, Predecessor: session.ID(), Successor: successor, Prompt: prompt}
}

// lastPersonPrompt is what the person last typed, without the console
// notes appended to it.
func lastPersonPrompt(messages []root.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == root.RoleUser && !strings.HasPrefix(string(messages[i].Content), "[AXLR") {
			return personTyped(string(messages[i].Content))
		}
	}
	return ""
}

// ContinuedNote is the console note the successor's first prompt carries.
func ContinuedNote(predecessor domain.SessionID, tokens, cap int) string {
	return fmt.Sprintf("[AXLR] This session continues session %s, whose context reached %d tokens (cap %d); its memory is recalled below and its transcript stays readable with /session.", predecessor, tokens, cap)
}

// firstRequest reports a session whose only person message is the one
// being asked: nothing earlier can be left behind.
func firstRequest(messages []root.Message) bool {
	prompts := 0
	for _, message := range messages {
		if message.Role == root.RoleUser && !strings.HasPrefix(string(message.Content), "[AXLR") {
			prompts++
		}
	}
	return prompts <= 1
}
