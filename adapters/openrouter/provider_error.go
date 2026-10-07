package openrouter

import "fmt"

type ProviderError struct {
	StatusCode int
	Category   ProviderErrorCategory
	// Provider names a non-OpenRouter endpoint; empty means OpenRouter.
	Provider string
}

func (e *ProviderError) Error() string {
	provider := e.Provider
	if provider == "" {
		provider = providerName
	}
	return fmt.Sprintf("%s %s (HTTP %d)", provider, e.Category, e.StatusCode)
}

func classifyProviderError(status int) *ProviderError {
	category := CategoryProviderFailure
	switch status {
	case 400, 404, 413, 422:
		category = CategoryInvalidRequest
	case 401, 403:
		category = CategoryAuthentication
	case 402:
		category = CategoryInsufficientCredits
	case 429:
		category = CategoryRateLimited
	}
	return &ProviderError{StatusCode: status, Category: category}
}
