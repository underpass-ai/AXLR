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
	mu sync.Mutex
	// saving orders saves, which run outside mu: a save takes a file lock
	// across processes, writes and syncs, and another model's estimate,
	// another session's in axlr-serve, does not wait for it.
	saving  sync.Mutex
	factors map[root.ModelID]float64
	// Memory keeps each model's factor across launches; nil keeps it for the
	// launch only. Before 20 November 2026 every launch started at 1, and
	// the first request of a resumed local session, numeric logs that Qwen
	// counts at 1.5 times o200k, went out at 93.5K tokens to a 90K server.
	Memory TokenScaleMemory
	asked  map[root.ModelID]bool
}

// TokenScaleMemory keeps a model's factor between launches.
type TokenScaleMemory interface {
	TokenFactor(model root.ModelID) (float64, bool)
	SaveTokenFactor(model root.ModelID, factor float64) error
}

// remembered loads the model's saved factor once per launch; t.mu is held.
func (t *TokenScale) remembered(model root.ModelID) {
	if t.Memory == nil || t.asked[model] {
		return
	}
	if t.asked == nil {
		t.asked = map[root.ModelID]bool{}
	}
	t.asked[model] = true
	if _, known := t.factors[model]; known {
		return
	}
	if factor, ok := t.Memory.TokenFactor(model); ok {
		if t.factors == nil {
			t.factors = map[root.ModelID]float64{}
		}
		t.factors[model] = min(max(factor, tokenScaleMinimum), tokenScaleMaximum)
	}
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
	t.remembered(model)
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
	t.remembered(model)
	if t.factors == nil {
		t.factors = map[root.ModelID]float64{}
	}
	if current, ok := t.factors[model]; ok {
		ratio = current*(1-tokenScaleWeight) + ratio*tokenScaleWeight
	}
	t.factors[model] = ratio
	t.mu.Unlock()
	if t.Memory == nil {
		return
	}
	t.saving.Lock()
	defer t.saving.Unlock()
	t.mu.Lock()
	latest := t.factors[model] // a later report may have moved it meanwhile
	t.mu.Unlock()
	// A factor that cannot be saved is learned again next launch.
	_ = t.Memory.SaveTokenFactor(model, latest)
}

// Estimate is the provider's likely count for a counted request.
func (t *TokenScale) Estimate(model root.ModelID, counted int) int {
	return int(float64(counted) * t.Factor(model))
}
