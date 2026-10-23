package openrouter

import "github.com/underpass-ai/AXLR/adapters/provider"

type ProviderErrorCategory = provider.ProviderErrorCategory

const (
	CategoryAuthentication      = provider.CategoryAuthentication
	CategoryInsufficientCredits = provider.CategoryInsufficientCredits
	CategoryRateLimited         = provider.CategoryRateLimited
	CategoryInvalidRequest      = provider.CategoryInvalidRequest
	CategoryProviderFailure     = provider.CategoryProviderFailure
)
