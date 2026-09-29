package openrouter

import "fmt"

type ProviderError struct {
	StatusCode int
	Category   ProviderErrorCategory
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("OpenRouter %s (HTTP %d)", e.Category, e.StatusCode)
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
