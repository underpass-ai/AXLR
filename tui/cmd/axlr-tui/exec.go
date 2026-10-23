package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
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

// execEvent is one line of --json: what the turn did, in order.
type execEvent struct {
	Type     string `json:"type"`
	Session  string `json:"session,omitempty"`
	Text     string `json:"text,omitempty"`
	Tool     string `json:"tool,omitempty"`
	Call     string `json:"call_id,omitempty"`
	Decision string `json:"decision,omitempty"`
	Status   string `json:"status,omitempty"`
	Error    string `json:"error,omitempty"`
}

// execTurn runs one prompt to its end without a person: the turn, the
// approvals the policy allows, and past the context cap the successor's
// turn, as the console continues it. It returns the session the turn ended
// in and the exit code.
func execTurn(ctx context.Context, start application.StartTurnUseCase, agent application.AgentTurnUseCase, session domain.Session, prompt string, asJSON bool, out, stderr io.Writer) (domain.Session, int) {
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
	text, err := root.NewText(prompt)
	if err != nil || text == "" {
		fmt.Fprintln(stderr, "axlr-tui: --exec needs a prompt")
		return session, execFailed
	}
	for successors := 0; ; successors++ {
		fmt.Fprintln(stderr, "axlr-tui: session", session.ID())
		write(execEvent{Type: "session", Session: string(session.ID())})
		err = start.Execute(ctx, &session, text, emit)
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
		session = capped.Successor
		if text, err = root.NewText(capped.Prompt); err != nil {
			break
		}
	}
	if !asJSON {
		_, _ = io.WriteString(out, "\n")
	}
	if err != nil {
		fmt.Fprintln(stderr, "axlr-tui:", err)
		write(execEvent{Type: "failed", Session: string(session.ID()), Error: err.Error()})
		return session, execFailed
	}
	if pending := session.Pending(); len(pending) > 0 {
		call := pending[0].Call
		fmt.Fprintf(stderr, "axlr-tui: %s waits for a decision on %s (%s); continue with --session %s in the console, or allow it with --approve\n", session.ID(), call.Name, call.ID, session.ID())
		write(execEvent{Type: "decision.required", Session: string(session.ID()), Tool: string(call.Name), Call: string(call.ID), Status: string(session.Status())})
		return session, execNeedsDecision
	}
	write(execEvent{Type: "completed", Session: string(session.ID()), Status: string(session.Status())})
	return session, execCompleted
}
