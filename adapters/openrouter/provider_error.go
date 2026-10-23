package openrouter

import "github.com/underpass-ai/AXLR/adapters/provider"

// ProviderError, ProviderHint and the hint constants are the shared ones of
// adapters/provider: the console reads them through this package, and every
// model adapter returns the same types.
type ProviderError = provider.ProviderError

type ProviderHint = provider.ProviderHint

const (
	HintNone        = provider.HintNone
	HintChooseModel = provider.HintChooseModel
	HintCheckKey    = provider.HintCheckKey
	HintCredits     = provider.HintCredits
	HintRetryLater  = provider.HintRetryLater
)

// MaxProviderMessageRunes bounds the provider's message kept in an error.
const MaxProviderMessageRunes = provider.MaxProviderMessageRunes

func classifyProviderError(status int) *ProviderError { return provider.ClassifyProviderError(status) }

func sanitizeProviderMessage(message string) string { return provider.SanitizeProviderMessage(message) }
