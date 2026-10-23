package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/adapters/contract"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
	"github.com/underpass-ai/AXLR/tui/dto"
)

// Exit codes of --exec: the turn finished, or it waits for a decision the
// run was not allowed to take (the session keeps the pending call for
// --session, or for the console).
const (
	execCompleted        = 0
	execFailed           = 1
	execNeedsDecision    = 3
	approveNone          = "none"
	approveLocal         = "local"
	approveAll           = "all"
	execMaximumSuccessor = 4
)

// execOutput is where --exec writes the answer or its JSON events.
var execOutput io.Writer = os.Stdout

// execPolicy is what a run without a person may approve beyond the
// settings' own rules: local tools (reads, writes and commands in the
// workspace, and the forged tools that are approved as they are), or
// every tool. The destructive list keeps its decision either way.
type execPolicy struct {
	next  application.ToolApprovalPolicyPort
	level string
}

func (p execPolicy) AutoApproves(id domain.ToolIdentity) bool {
	switch {
	case p.level == approveAll:
		return true
	case p.level == approveLocal && id.Kind == domain.ToolKindLocal:
		return true
	}
	return p.next != nil && p.next.AutoApproves(id)
}

func (p execPolicy) AlwaysAsk() []string {
	if always, ok := p.next.(application.AlwaysAskPort); ok {
		return always.AlwaysAsk()
	}
	return nil
}

// execModes are the work modes a run may take: the ones that only tailor
// tool policy. The ceremony modes need the console's cards.
func execMode(raw string) (domain.WorkMode, error) {
	if raw == "" {
		return domain.ModeNormal, nil
	}
	mode, err := domain.ParseWorkMode(raw)
	if err != nil {
		return "", err
	}
	switch mode {
	case domain.ModeNormal, domain.ModeReview, domain.ModeWriter, domain.ModeResearch:
		return mode, nil
	}
	return "", fmt.Errorf("--mode %s needs the console; --exec takes normal, review, writer or research", mode)
}

// execEvent is one line of --json: what the turn did, in order. The last
// line, result, carries the execution's result (docs/runtime-contract.md):
// a program reads it without reading the lines before.
type execEvent struct {
	Type     string               `json:"type"`
	Session  string               `json:"session,omitempty"`
	Text     string               `json:"text,omitempty"`
	Tool     string               `json:"tool,omitempty"`
	Call     string               `json:"call_id,omitempty"`
	Decision string               `json:"decision,omitempty"`
	Status   string               `json:"status,omitempty"`
	Error    string               `json:"error,omitempty"`
	Result   *dto.ExecutionResult `json:"result,omitempty"`
}

// execStep is the first step of a run: the prompt's turn, or the decision
// on the session's pending call. Past the context cap, every later step is
// the successor's turn with the capped prompt.
type execStep func(context.Context, *domain.Session, func(application.Event) error) error

// execTurn runs one prompt to its end without a person: the turn, the
// approvals the policy allows, and past the context cap the successor's
// turn, as the console continues it. It returns the session the turn ended
// in and the exit code.
func execTurn(ctx context.Context, start application.StartTurnUseCase, agent application.AgentTurnUseCase, usage application.SessionUsagePort, session domain.Session, prompt string, asJSON bool, out, stderr io.Writer) (domain.Session, int) {
	text, err := root.NewText(prompt)
	if err != nil || text == "" {
		fmt.Fprintln(stderr, "axlr-tui: --exec needs a prompt")
		return session, execFailed
	}
	first := func(ctx context.Context, s *domain.Session, emit func(application.Event) error) error {
		return start.Execute(ctx, s, text, emit)
	}
	return execLoop(ctx, first, start, agent, usage, session, asJSON, out, stderr)
}

// execDecision decides the pending call a run left in a saved session, as
// the console's card would, and runs the turn to its end. Only the first
// pending call can be decided; --call names it to make sure it is the one.
func execDecision(ctx context.Context, resolve application.ResolveToolUseCase, start application.StartTurnUseCase, agent application.AgentTurnUseCase, usage application.SessionUsagePort, session domain.Session, call string, decision domain.ToolDecision, asJSON bool, out, stderr io.Writer) (domain.Session, int) {
	pending := session.Pending()
	if len(pending) == 0 {
		fmt.Fprintf(stderr, "axlr-tui: session %s has no pending call to decide\n", session.ID())
		return session, execFailed
	}
	if call != "" && string(pending[0].Call.ID) != call {
		fmt.Fprintf(stderr, "axlr-tui: the pending call of %s is %s (%s), not %s\n", session.ID(), pending[0].Call.Name, pending[0].Call.ID, call)
		return session, execFailed
	}
	id := pending[0].Call.ID
	first := func(ctx context.Context, s *domain.Session, emit func(application.Event) error) error {
		if s.Status() == domain.StatusInterrupted {
			if err := s.ResumePending(); err != nil {
				return err
			}
		}
		return resolve.Execute(ctx, s, id, decision, emit)
	}
	return execLoop(ctx, first, start, agent, usage, session, asJSON, out, stderr)
}

// execLoop drives one execution: its first step, then the agent's loop until
// an answer or a decision, and past the context cap the successor's turn.
// It ends by writing the result and returns the session the execution
// ended in and the exit code.
func execLoop(ctx context.Context, first execStep, start application.StartTurnUseCase, agent application.AgentTurnUseCase, usage application.SessionUsagePort, session domain.Session, asJSON bool, out, stderr io.Writer) (domain.Session, int) {
	encoder := json.NewEncoder(out)
	write := func(event execEvent) {
		if asJSON {
			_ = encoder.Encode(event)
		}
	}
	emit := func(e application.Event) error {
		switch e.Kind {
		case application.EventTextDelta:
			if asJSON {
				write(execEvent{Type: "text", Text: string(e.Text)})
			} else {
				_, _ = io.WriteString(out, string(e.Text))
			}
		case application.EventReasoningProgress:
			if e.ReasoningText != "" {
				write(execEvent{Type: "reasoning", Text: e.ReasoningText})
			}
		case application.EventToolActivity:
			kind := "tool.requested"
			if e.Tool.Outcome != nil {
				kind = "tool.completed"
			}
			write(execEvent{Type: kind, Tool: string(e.Tool.Call.Name), Call: string(e.Tool.Call.ID), Decision: string(e.Tool.Decision)})
		}
		return nil
	}
	var err error
	var predecessors []domain.SessionID
	step := first
	for successors := 0; ; successors++ {
		fmt.Fprintln(stderr, "axlr-tui: session", session.ID())
		write(execEvent{Type: "session", Session: string(session.ID())})
		err = step(ctx, &session, emit)
		if err == nil && (session.Status() == domain.StatusStreaming || session.Status() == domain.StatusApproval) {
			err = agent.Execute(ctx, &session, emit)
		}
		capped, ok := application.IsContextCap(err)
		if !ok {
			break
		}
		if successors == execMaximumSuccessor {
			err = errors.New("the prompt kept reaching the context cap in successor sessions")
			break
		}
		fmt.Fprintln(stderr, "axlr-tui:", capped.Error())
		write(execEvent{Type: "session.continued", Session: string(capped.Successor.ID())})
		predecessors = append(predecessors, session.ID())
		session = capped.Successor
		text, textErr := root.NewText(capped.Prompt)
		if textErr != nil {
			err = textErr
			break
		}
		step = func(ctx context.Context, s *domain.Session, emit func(application.Event) error) error {
			return start.Execute(ctx, s, text, emit)
		}
	}
	if !asJSON {
		_, _ = io.WriteString(out, "\n")
	}
	result := domain.NewExecutionResult(session, err)
	result.Spanned(predecessors...)
	if usage != nil {
		for _, id := range result.Sessions {
			if ledger, loadErr := usage.LoadUsage(context.WithoutCancel(ctx), id); loadErr == nil {
				result.AddUsage(ledger)
			}
		}
	}
	wire := contract.ExecutionResult(result)
	code := execFailed
	switch result.Status {
	case domain.ExecutionCompleted:
		code = execCompleted
		write(execEvent{Type: "completed", Session: string(session.ID()), Status: string(session.Status())})
	case domain.ExecutionNeedsDecision:
		code = execNeedsDecision
		call := *result.Pending
		fmt.Fprintf(stderr, "axlr-tui: %s waits for a decision on %s (%s); continue with --session %s in the console, decide it with --session %s --decide approve, or allow it with --approve\n", session.ID(), call.Name, call.ID, session.ID(), session.ID())
		write(execEvent{Type: "decision.required", Session: string(session.ID()), Tool: string(call.Name), Call: string(call.ID), Status: string(session.Status())})
	default:
		fmt.Fprintln(stderr, "axlr-tui:", result.Error)
		write(execEvent{Type: "failed", Session: string(session.ID()), Error: result.Error})
	}
	write(execEvent{Type: "result", Session: string(session.ID()), Result: &wire})
	return session, code
}
