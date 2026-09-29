package openrouter

type ProviderErrorCategory string

const (
	CategoryAuthentication      ProviderErrorCategory = "authentication"
	CategoryInsufficientCredits ProviderErrorCategory = "insufficient_credits"
	CategoryRateLimited         ProviderErrorCategory = "rate_limited"
	CategoryInvalidRequest      ProviderErrorCategory = "invalid_request"
	CategoryProviderFailure     ProviderErrorCategory = "provider_failure"
)
