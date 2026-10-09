package domain

import "math"

// The prompt budget assumes the 2.44 bytes a prompt token held on Claude
// Haiku 5.5 (8 October 2026). z-ai/glm-5.3-flash held about 4.2 that day, so
// its requests were cut at about 37K tokens instead of the 64K the budget
// aims at. The console therefore measures each request: the bytes of the
// body it sent against the prompt tokens the provider counted.
const (
	// MinimumBytesPerToken and MaximumBytesPerToken bound a ratio, in
	// hundredths of a byte: outside 1.5 to 6 a sample says more about the
	// request than about the model's tokenizer.
	MinimumBytesPerToken = 150
	MaximumBytesPerToken = 600
	// CalibrationSamples is how many requests a model needs before its
	// measured ratio replaces the default.
	CalibrationSamples = 5
	// calibrationWindow bounds the moving average: past it a new request
	// weighs a fiftieth, so the ratio follows a provider's tokenizer
	// within tens of requests while one odd request barely moves it.
	calibrationWindow = 50
	// minimumCalibrationTokens leaves out prompts too small to measure:
	// below it the fixed overhead of the request weighs on the ratio.
	minimumCalibrationTokens = 2048
)

// BytesPerToken is what a model's prompt tokens measured: Ratio bytes per
// token, a moving average over Samples requests.
type BytesPerToken struct {
	Ratio   float64
	Samples int
}

// Observe adds one request: the size of its body and the prompt tokens the
// provider counted for it. A prompt too small to measure changes nothing.
func (b BytesPerToken) Observe(requestBytes, promptTokens int) BytesPerToken {
	if requestBytes <= 0 || promptTokens < minimumCalibrationTokens {
		return b
	}
	sample := math.Min(math.Max(float64(requestBytes)/float64(promptTokens), MinimumBytesPerToken/100.0), MaximumBytesPerToken/100.0)
	if b.Samples <= 0 || !b.valid() {
		return BytesPerToken{Ratio: sample, Samples: 1}
	}
	weight := float64(min(b.Samples+1, calibrationWindow))
	return BytesPerToken{Ratio: b.Ratio + (sample-b.Ratio)/weight, Samples: b.Samples + 1}
}

// Hundredths is the ratio in hundredths of a byte, the unit of
// ContextBudgetForPromptAt, once enough requests were measured.
func (b BytesPerToken) Hundredths() (int, bool) {
	if b.Samples < CalibrationSamples || !b.valid() {
		return 0, false
	}
	return int(math.Round(b.Ratio * 100)), true
}

func (b BytesPerToken) valid() bool {
	return b.Ratio >= MinimumBytesPerToken/100.0 && b.Ratio <= MaximumBytesPerToken/100.0
}
