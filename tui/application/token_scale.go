package application

import (
	"sync"

	root "github.com/underpass-ai/AXLR/domain"
)

// TokenScale learns, per model, how the provider's prompt token count
// relates to the console's o200k count of the same request: on 13 October
// 2026 claude-haiku-5.5 through OpenRouter billed 27K prompt tokens where
// o200k counted 9.4K of Spanish text. The cap check multiplies the count
// by the factor, which starts at 1 and follows the reports with an
// exponential average, clamped between 1 and 4.
type TokenScale struct {
	mu      sync.Mutex
	factors map[root.ModelID]float64
}

const (
	tokenScaleMinimum = 1.0
	tokenScaleMaximum = 4.0
	tokenScaleWeight  = 0.5
)

// Factor is the model's current factor.
func (t *TokenScale) Factor(model root.ModelID) float64 {
	if t == nil {
		return 1
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if factor, ok := t.factors[model]; ok {
		return factor
	}
	return 1
}

// Observe records one request's counted and reported prompt tokens.
func (t *TokenScale) Observe(model root.ModelID, counted, reported int) {
	if t == nil || counted <= 0 || reported <= 0 {
		return
	}
	ratio := min(max(float64(reported)/float64(counted), tokenScaleMinimum), tokenScaleMaximum)
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.factors == nil {
		t.factors = map[root.ModelID]float64{}
	}
	if current, ok := t.factors[model]; ok {
		ratio = current*(1-tokenScaleWeight) + ratio*tokenScaleWeight
	}
	t.factors[model] = ratio
}

// Estimate is the provider's likely count for a counted request.
func (t *TokenScale) Estimate(model root.ModelID, counted int) int {
	return int(float64(counted) * t.Factor(model))
}
