package application

import (
	"context"
	"errors"
	"strings"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type ContinueTurnUseCase struct {
	Models      ModelStreamPort
	Store       SessionStorePort
	Diagnostics DiagnosticPort
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
	_, contextSpan := StartDiagnosticSpan(ctx, u.Diagnostics, DiagnosticActionContext, DiagnosticEvent{Messages: len(session.Messages()), Tools: len(session.ToolSnapshot())})
	req := root.CompletionRequest{Model: session.Export().Model, Messages: modelMessages(session)}
	snapshot := session.ToolSnapshot()
	for _, tool := range snapshot {
		req.Tools = append(req.Tools, tool.Definition)
	}
	if err := req.Validate(); err != nil {
		contextSpan.End(DiagnosticErrorInvalidState)
		return interrupt(err)
	}
	contextSpan.End(DiagnosticErrorNone)
	if err := emit(Event{Kind: EventStreamStart, MessageCount: len(session.Messages())}); err != nil {
		return interrupt(err)
	}
	if err := emit(Event{Kind: EventState, State: domain.StatusStreaming}); err != nil {
		return interrupt(err)
	}
	started := time.Now()
	modelCtx, modelSpan := StartDiagnosticSpan(ctx, u.Diagnostics, DiagnosticActionModel, DiagnosticEvent{Messages: len(req.Messages), Tools: len(req.Tools)})
	if u.Diagnostics != nil {
		_ = u.Diagnostics.Record(DiagnosticEvent{Stage: DiagnosticProviderStart, SpanID: CurrentDiagnosticSpan(modelCtx)})
	}
	chunks, bytes := 0, 0
	result, err := u.Models.Stream(modelCtx, req, func(delta root.Text) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := root.NewText(string(delta)); err != nil {
			return err
		}
		draft.WriteString(string(delta))
		chunks++
		bytes += len(delta)
		if u.Diagnostics != nil {
			_ = u.Diagnostics.Record(DiagnosticEvent{Stage: DiagnosticProviderProgress, SpanID: CurrentDiagnosticSpan(modelCtx), Chunks: chunks, Bytes: bytes, ElapsedMilliseconds: time.Since(started).Milliseconds()})
		}
		return emit(Event{Kind: EventTextDelta, Text: delta})
	})
	class := DiagnosticErrorNone
	streamErr := errors.Join(err, ctx.Err())
	if streamErr != nil {
		class = DiagnosticErrorProvider
		if errors.Is(streamErr, context.DeadlineExceeded) {
			class = DiagnosticErrorTimeout
		} else if errors.Is(streamErr, context.Canceled) {
			class = DiagnosticErrorCancelled
		}
	}
	modelSpan.End(class)
	if u.Diagnostics != nil {
		_ = u.Diagnostics.Record(DiagnosticEvent{Stage: DiagnosticProviderDone, SpanID: CurrentDiagnosticSpan(modelCtx), Chunks: chunks, Bytes: bytes, ElapsedMilliseconds: time.Since(started).Milliseconds(), ErrorClass: class})
	}
	if err != nil {
		return interrupt(err)
	}
	if err := ctx.Err(); err != nil {
		return interrupt(err)
	}
	if len(result.Message.ToolCalls) > 0 && u.Diagnostics != nil {
		_ = u.Diagnostics.Record(DiagnosticEvent{Stage: DiagnosticToolRequested, SpanID: CurrentDiagnosticSpan(modelCtx), Chunks: len(result.Message.ToolCalls)})
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
	if err := emitSession(session, emit); err != nil {
		return err
	}
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
		if err := rejectUnknownCall(ctx, session, u.Store, u.Diagnostics, pending); err != nil {
			return err
		}
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
