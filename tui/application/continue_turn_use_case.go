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
	// Context projects the transcript; nil derives the budget from Windows.
	Context ModelContextPort
	// Windows sizes the default projection to the session model: its window
	// when known, the prompt budget otherwise; nil uses the default prompt
	// budget.
	Windows        ModelContextWindowPort
	Validation     ToolArgumentValidationPort
	Models         ModelStreamPort
	Store          SessionStorePort
	Diagnostics    DiagnosticPort
	PluginGuidance func(context.Context) (string, error)
	PluginSkills   PluginSkillPort
	SessionLabels  SessionLabelsPort
	// Ceremonies drives MADE for the debug and delivery modes; nil without MADE.
	Ceremonies *CeremonyDriver
	// SelfRepair serves the model's repair requests; nil hides nothing but
	// refuses the request with the reason.
	SelfRepair RepairRequestPort
	// Judge enables TypeSafe Jev; nil, the default, offers no axlr_judge and
	// runs no final check.
	Judge *Judge
	// Calibration learns each model's bytes per prompt token from the
	// requests it serves; nil measures nothing.
	Calibration TokenCalibrationPort
	// TurnToolCalls is the turn's tool-call budget (settings'
	// turn_tool_calls); zero means domain.MaxTurnToolCalls.
	TurnToolCalls int
	// Usage keeps each session's ledger of tokens, cost and latency, and
	// MaxSessionUSD refuses a session's requests once its known cost
	// reaches it (zero, no limit); nil keeps no ledger (session_usage.go).
	Usage         SessionUsagePort
	MaxSessionUSD float64
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
				return joinCause(cause, err)
			}
		}
		// The caller's cancelled stream must not cancel saving its interrupted draft.
		if err := u.Store.Save(context.WithoutCancel(ctx), next); err != nil {
			return joinCause(cause, err)
		}
		*session = next
		return joinCause(cause, emit(Event{Kind: EventState, State: next.Status()}))
	}
	if err := ctx.Err(); err != nil {
		return interrupt(err)
	}
	// What the person wrote during the last tool step joins this turn here,
	// before the next request is built, rather than cancelling it.
	if steered, err := applySteer(ctx, session, u.Store, u.Diagnostics); err != nil {
		return interrupt(err)
	} else if steered {
		if err := emitSession(session, emit); err != nil {
			return interrupt(err)
		}
	}
	if err := warnCallBudget(ctx, session, u, emit); err != nil {
		return interrupt(err)
	}
	contextCtx, contextSpan := StartDiagnosticSpan(ctx, u.Diagnostics, DiagnosticActionContext, DiagnosticEvent{Messages: len(session.Messages()), Tools: len(session.ToolSnapshot())})
	hostTools := HostTools()
	if u.Judge.offersTool() {
		hostTools = append(hostTools, JudgeTool())
	}
	if err := session.EnsureHostTools(hostTools); err != nil {
		contextSpan.End(DiagnosticErrorInvalidState)
		return interrupt(err)
	}
	model := sessionModel(*session)
	messages := session.Messages()
	if _, live := session.Ceremony(); !live {
		messages = markSteered(messages)
	}
	projector := u.Context
	if projector == nil {
		sized, err := NewModelContextProjector(projectionBudget(u.Windows, *session))
		if err != nil {
			sized, _ = NewModelContextProjector(domain.ContextBudgetForWindow(0))
		}
		// A compact step starts from the ledger; references to the
		// transcript stay absolute through the origin table.
		if ledgered, origin := ledgerProjection(*session, messages); origin != nil {
			messages, sized = ledgered, sized.WithOrigin(origin)
		}
		projector = sized
	}
	projection, err := projector.Project(messages)
	if replaced := len(session.Messages()) - len(messages) + 1; len(messages) != len(session.Messages()) && err == nil {
		projection.OriginalMessages = len(session.Messages())
		projection.DroppedMessages += replaced
	}
	if err != nil {
		contextSpan.End(DiagnosticErrorInvalidState)
		return interrupt(err)
	}
	snapshot := session.ToolSnapshot()
	guidance := modelHostGuidance(session)
	_, _, focused := focusedRun(*session)
	if u.SessionLabels != nil && !focused {
		text, err := sessionContextGuidance(ctx, *session, u.SessionLabels)
		if err != nil {
			contextSpan.End(DiagnosticErrorInvalidState)
			return interrupt(err)
		}
		guidance.Content += root.Text(text)
	}
	if u.PluginGuidance != nil && !focused {
		pluginText, err := u.PluginGuidance(ctx)
		if err != nil {
			contextSpan.End(DiagnosticErrorInvalidState)
			return interrupt(err)
		}
		guidance.Content = root.Text(string(guidance.Content) + pluginText)
	}
	tools := SessionTools(*session, snapshot)
	if !u.Judge.offersTool() {
		// A session that once had Jev keeps axlr_judge in its snapshot; it is
		// absent from the request while Jev is off.
		tools = withoutTool(tools, HostJudgeName)
	}
	req := root.CompletionRequest{Model: model, Messages: append([]root.Message{guidance}, projection.Messages...), Tools: tools}
	if u.Diagnostics != nil {
		_ = u.Diagnostics.Record(DiagnosticEvent{Stage: DiagnosticContextProjected, SpanID: CurrentDiagnosticSpan(contextCtx), Messages: len(req.Messages), Tools: len(req.Tools), OriginalMessages: projection.OriginalMessages, DroppedMessages: projection.DroppedMessages, OriginalBytes: projection.OriginalBytes, ProjectedBytes: projection.ProjectedBytes, ContextCutIndex: projection.CutIndex})
	}
	if err := req.Validate(); err != nil {
		contextSpan.End(DiagnosticErrorInvalidState)
		return interrupt(err)
	}
	contextSpan.End(DiagnosticErrorNone)
	if err := u.budgetRefusal(ctx, *session); err != nil {
		return interrupt(err)
	}
	if err := emit(Event{Kind: EventStreamStart, MessageCount: len(session.Messages())}); err != nil {
		return interrupt(err)
	}
	if err := emit(Event{Kind: EventState, State: domain.StatusStreaming}); err != nil {
		return interrupt(err)
	}
	started := time.Now()
	modelCtx, modelSpan := StartDiagnosticSpan(ctx, u.Diagnostics, DiagnosticActionModel, DiagnosticEvent{Messages: len(req.Messages), Tools: len(req.Tools)})
	modelCtx = WithProviderActivity(modelCtx, func(phase domain.ProviderPhase) { _ = emit(Event{Kind: EventProviderActivity, ProviderPhase: phase}) })
	modelCtx = WithToolCallProgress(modelCtx, func(name string, bytes int) {
		_ = emit(Event{Kind: EventToolCallProgress, ToolCallName: name, ToolCallBytes: bytes})
	})
	meter := &usageMeter{started: started}
	modelCtx = meter.watch(modelCtx)
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
		meter.mark()
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
		done := DiagnosticEvent{Stage: DiagnosticProviderDone, SpanID: CurrentDiagnosticSpan(modelCtx), Chunks: chunks, Bytes: bytes, ElapsedMilliseconds: time.Since(started).Milliseconds(), ErrorClass: class}
		if usage := result.Usage; usage != nil {
			done.PromptTokens, done.CompletionTokens, done.CachedTokens, done.CacheWriteTokens = usage.PromptTokens, usage.CompletionTokens, usage.CachedTokens, usage.CacheWriteTokens
			done.ReasoningTokens, done.CostUSD = usage.ReasoningTokens, usage.Cost
		}
		_ = u.Diagnostics.Record(done)
	}
	if err != nil {
		return interrupt(err)
	}
	if err := ctx.Err(); err != nil {
		return interrupt(err)
	}
	u.recordUsage(ctx, *session, req.Model, result, meter, emit)
	if u.Calibration != nil && result.Usage != nil && result.RequestBytes > 0 {
		// A measurement that cannot be saved is only one sample lost.
		_ = u.Calibration.Observe(context.WithoutCancel(ctx), req.Model, result.RequestBytes, result.Usage.PromptTokens)
	}
	if len(result.Message.ToolCalls) > 0 && u.Diagnostics != nil {
		_ = u.Diagnostics.Record(DiagnosticEvent{Stage: DiagnosticToolRequested, SpanID: CurrentDiagnosticSpan(modelCtx), Chunks: len(result.Message.ToolCalls)})
	}
	next := *session
	if err := next.CompleteAssistantWithin(result, u.turnLimit()); err != nil {
		// CompleteAssistant can deliberately pause at the call limit: the
		// answer is kept with its calls answered as not run, so the person
		// sees it in the transcript instead of a dropped draft.
		if next.Status() == domain.StatusInterrupted {
			if saveErr := u.Store.Save(context.WithoutCancel(ctx), next); saveErr != nil {
				return errors.Join(err, saveErr)
			}
			*session = next
			return errors.Join(err, emitSession(session, emit), emit(Event{Kind: EventState, State: next.Status()}))
		}
		return interrupt(err)
	}
	if err := u.Store.Save(ctx, next); err != nil {
		return err
	}
	*session = next
	// Reject only at the head: tool results must retain model order. Decision
	// resolution must apply this lookup again as later calls reach the head.
	if err := rejectUnknown(ctx, session, u.Store, func(Event) error { return nil }, u.Diagnostics); err != nil {
		return err
	}
	if err := emitSession(session, emit); err != nil {
		return err
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
