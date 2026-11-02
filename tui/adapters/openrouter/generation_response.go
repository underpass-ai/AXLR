package openrouter

import (
	"math"

	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// generationResponse is what the generation endpoint answers, of which the
// cost and the tokens are read; other fields are ignored and the optional
// ones may be null. Numbers are read as floats so a count written 12.0
// still reads.
type generationResponse struct {
	Data *struct {
		// TotalCost is what OpenRouter charged, in dollars.
		TotalCost *float64 `json:"total_cost"`
		// IsBYOK marks a request served with the person's own provider
		// key: TotalCost is then OpenRouter's fee, and the provider bills
		// UpstreamInferenceCost to that key, as in a stream's usage.
		IsBYOK                 bool     `json:"is_byok"`
		UpstreamInferenceCost  *float64 `json:"upstream_inference_cost"`
		TokensPrompt           *float64 `json:"tokens_prompt"`
		TokensCompletion       *float64 `json:"tokens_completion"`
		NativeTokensPrompt     *float64 `json:"native_tokens_prompt"`
		NativeTokensCompletion *float64 `json:"native_tokens_completion"`
	} `json:"data"`
}

// cost is the request's cost and tokens: the provider's own token counts
// when given, OpenRouter's normalized ones otherwise. Stats without a cost
// are not final yet.
func (r generationResponse) cost() (domain.GenerationCost, error) {
	data := r.Data
	if data == nil || data.TotalCost == nil {
		return domain.GenerationCost{}, application.ErrGenerationPending
	}
	cost := *data.TotalCost
	if data.IsBYOK && data.UpstreamInferenceCost != nil {
		cost += *data.UpstreamInferenceCost
	}
	if cost < 0 || math.IsNaN(cost) || math.IsInf(cost, 0) {
		return domain.GenerationCost{}, errMalformedGeneration
	}
	prompt, promptKnown := generationTokens(data.NativeTokensPrompt, data.TokensPrompt)
	completion, completionKnown := generationTokens(data.NativeTokensCompletion, data.TokensCompletion)
	return domain.GenerationCost{Cost: cost, PromptTokens: prompt, CompletionTokens: completion, TokensKnown: promptKnown && completionKnown}, nil
}

// generationTokens is the first count given that is a sensible number of
// tokens, false without one.
func generationTokens(counts ...*float64) (int, bool) {
	for _, count := range counts {
		if count != nil && *count >= 0 && *count < math.MaxInt32 {
			return int(*count), true
		}
	}
	return 0, false
}
