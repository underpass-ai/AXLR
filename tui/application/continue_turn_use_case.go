package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type ContinueTurnUseCase struct {
	Models ModelStreamPort
	Store  SessionStorePort
}

func (u ContinueTurnUseCase) Execute(ctx context.Context, session *domain.Session, emit func(Event) error) error {
	if session == nil || u.Models == nil || u.Store == nil {
		return errors.New("continue turn requires session, model and store")
	}
	if session.Status() != domain.StatusStreaming || len(session.Pending()) != 0 {
		return errors.New("continue turn requires streaming state without pending calls")
	}
	if emit == nil {
		emit = func(Event) error { return nil }
	}
	var draft strings.Builder
	interrupt := func(cause error) error {
		next := *session
		if next.Status() == domain.StatusStreaming {
			if err := next.InterruptDraft(root.Text(draft.String())); err != nil {
				return errors.Join(cause, err)
			}
		}
		// The caller's cancelled stream must not cancel saving its interrupted draft.
		if err := u.Store.Save(context.WithoutCancel(ctx), next); err != nil {
			return errors.Join(cause, err)
		}
		*session = next
		return errors.Join(cause, emit(Event{Kind: EventState, State: next.Status()}))
	}
	if err := ctx.Err(); err != nil {
		return interrupt(err)
	}
	req := root.CompletionRequest{Model: session.Export().Model, Messages: session.Messages()}
	snapshot := session.ToolSnapshot()
	for _, tool := range snapshot {
		req.Tools = append(req.Tools, tool.Definition)
	}
	if err := req.Validate(); err != nil {
		return interrupt(err)
	}
	if err := emit(Event{Kind: EventState, State: domain.StatusStreaming}); err != nil {
		return interrupt(err)
	}
	result, err := u.Models.Stream(ctx, req, func(delta root.Text) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := root.NewText(string(delta)); err != nil {
			return err
		}
		draft.WriteString(string(delta))
		return emit(Event{Kind: EventTextDelta, Text: delta})
	})
	if err != nil {
		return interrupt(err)
	}
	if err := ctx.Err(); err != nil {
		return interrupt(err)
	}
	next := *session
	if err := next.CompleteAssistant(result); err != nil {
		// CompleteAssistant can deliberately pause at the call limit.
		if next.Status() == domain.StatusInterrupted {
			if saveErr := u.Store.Save(context.WithoutCancel(ctx), next); saveErr != nil {
				return errors.Join(err, saveErr)
			}
			*session = next
			return errors.Join(err, emit(Event{Kind: EventState, State: next.Status()}))
		}
		return interrupt(err)
	}
	if err := u.Store.Save(ctx, next); err != nil {
		return err
	}
	*session = next
	// Reject only at the head: tool results must retain model order. Decision
	// resolution must apply this lookup again as later calls reach the head.
	for len(session.Pending()) > 0 {
		pending := session.Pending()[0]
		known := false
		for _, tool := range snapshot {
			if tool.Definition.Name == pending.Call.Name {
				known = true
				break
			}
		}
		if known {
			break
		}
		next = *session
		outcome := domain.ToolOutcome{Content: root.Text(fmt.Sprintf("unknown tool %q rejected", pending.Call.Name)), IsError: true}
		if err := next.RecordToolOutcome(pending.Call.ID, domain.DecisionDeny, outcome); err != nil {
			return err
		}
		if err := u.Store.Save(ctx, next); err != nil {
			return err
		}
		*session = next
	}
	// Export clones outcomes as well as slices so callbacks cannot alter history.
	activity := session.Export().Activity
	for _, call := range result.Message.ToolCalls {
		for _, record := range activity {
			if record.Call.ID == call.ID {
				if err := emit(Event{Kind: EventToolActivity, Tool: record}); err != nil {
					return err
				}
				break
			}
		}
	}
	return emit(Event{Kind: EventState, State: session.Status(), Usage: result.Usage})
}
