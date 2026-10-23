package openrouter

import (
	"io"

	"github.com/underpass-ai/AXLR/adapters/provider"
)

const (
	maxResponseBytes = provider.MaxResponseBytes
	maxSSEEventBytes = provider.MaxSSEEventBytes
)

// newSSEDecoder is the shared decoder with OpenRouter's name on its limits:
// a client for another endpoint labels them with the endpoint's.
func newSSEDecoder(reader io.Reader) *provider.SSEDecoder {
	return provider.NewSSEDecoder(reader, providerName)
}
