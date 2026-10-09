package openrouter

import (
	"encoding/json"
	"fmt"
)

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

// errorBodyStatus reads the error object a provider sends in a body it
// answered with HTTP 200, in a stream event or a whole reply: its code is
// the status to classify, 502 when it has none. The message is not kept.
func errorBodyStatus(raw json.RawMessage) (int, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, false
	}
	var provider struct {
		Code int `json:"code"`
	}
	_ = json.Unmarshal(raw, &provider)
	if provider.Code == 0 {
		return 502, true
	}
	return provider.Code, true
}
