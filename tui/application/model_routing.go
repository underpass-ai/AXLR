package application

import (
	"context"
	"errors"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// routingOf is the routing that chooses the model of the session's next
// request: the execution's, unless a ceremony asks a model of its own,
// such as the planner, which it keeps.
func routingOf(s domain.Session) (domain.ExecutionRouting, bool) {
	routing, has := s.Routing()
	if !has {
		return domain.ExecutionRouting{}, false
	}
	if run, live := s.Ceremony(); live && run.Model != "" {
		return domain.ExecutionRouting{}, false
	}
	return routing, true
}

// rankRoutes ranks the routing's candidates for a request of counted
// tokens: what the console knows of each, the request's size as each is
// estimated to count it, the routing's narrowing of each cap, and each
// model's recent requests. A store that cannot be read ranks on priors.
func (u ContinueTurnUseCase) rankRoutes(ctx context.Context, routing domain.ExecutionRouting, counted int) (domain.RouteRanking, error) {
	if u.Routing == nil {
		return domain.RouteRanking{}, errors.New("this console cannot route an execution's requests: it knows nothing of the candidate models")
	}
	now := time.Now()
	models := routing.Candidates()
	stats := map[root.ModelID]domain.ModelStats{}
	if u.ModelStats != nil {
		if read, err := u.ModelStats.ModelStats(ctx, models, now); err == nil {
			stats = read
		}
	}
	candidates := make([]domain.RouteCandidate, 0, len(models))
	for _, model := range models {
		candidate := u.Routing.RouteFacts(ctx, model)
		candidate.Model = model
		candidate.Cap = routing.Cap(candidate.Cap)
		candidate.Tokens = u.Scale.Estimate(model, counted)
		candidate.Stats = stats[model]
		candidates = append(candidates, candidate)
	}
	return domain.RankRoutes(routing, candidates, now), nil
}

// estimatedRequestTokens sizes a request for routing without a token
// counter: its bytes at the default ratio.
func estimatedRequestTokens(guidance root.Message, tools []root.ToolDefinition, messages []root.Message) int {
	bytes := len(guidance.Content)
	for _, tool := range tools {
		bytes += len(tool.Name) + len(tool.Description) + len(tool.Parameters.Bytes())
	}
	for _, message := range messages {
		bytes += len(message.Content) + len(message.Reasoning.Bytes())
		for _, call := range message.ToolCalls {
			bytes += len(call.Name) + len(call.Arguments.Bytes())
		}
	}
	return domain.EstimatedTokens(bytes, 0)
}

// fallsBack reports whether a failed request may be asked again of the
// next candidate: the provider failed it or timed out before showing any
// sign, no text, no reasoning and no call reached the person, and neither
// the caller nor the execution's budget stopped it.
func fallsBack(ctx context.Context, err error, meter *usageMeter, drafted int) bool {
	if err == nil || ctx.Err() != nil || budgetCause(ctx, err) != nil {
		return false
	}
	return drafted == 0 && meter.first.Load() == 0 && meter.streamed() == 0
}

// observeModel adds a request that finished or that the provider failed
// to its model's stats; a request the caller cancelled says nothing of
// the model. A store that cannot be written loses one observation.
func (u ContinueTurnUseCase) observeModel(ctx context.Context, model root.ModelID, sample domain.UsageSample, streamErr error) {
	if u.ModelStats == nil || ctx.Err() != nil {
		return
	}
	_ = u.ModelStats.ObserveModel(context.WithoutCancel(ctx), model, domain.ObservationOf(sample, streamErr != nil, time.Now()))
}
