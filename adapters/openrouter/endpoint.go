package openrouter

import "github.com/underpass-ai/AXLR/adapters/provider"

const providerName = provider.DefaultProviderName

// IsLoopbackEndpoint reports whether raw is an absolute http(s) URL whose host
// is localhost or a loopback address.
func IsLoopbackEndpoint(raw string) bool { return provider.IsLoopbackEndpoint(raw) }

func validateEndpoint(raw string) error { return provider.ValidateEndpoint(raw) }

func endpointName(raw string) string { return provider.EndpointName(raw) }
