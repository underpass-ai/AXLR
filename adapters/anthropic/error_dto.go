package anthropic

import (
	"encoding/json"

	"github.com/underpass-ai/AXLR/adapters/provider"
)

// errorDTO is the error object Anthropic sends in a body or a stream event:
// {"type":"error","error":{"type":"overloaded_error","message":"…"}}.
type errorDTO struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// errorStatuses are the HTTP statuses Anthropic's error types stand for,
// for a stream event that carries no status.
var errorStatuses = map[string]int{
	"invalid_request_error": 400,
	"authentication_error":  401,
	"permission_error":      403,
	"not_found_error":       404,
	"request_too_large":     413,
	"rate_limit_error":      429,
	"api_error":             500,
	"overloaded_error":      529,
}

// providerError reads an error object; false when raw holds none.
func providerError(raw json.RawMessage) (*provider.ProviderError, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, false
	}
	var wire errorDTO
	_ = json.Unmarshal(raw, &wire)
	status := errorStatuses[wire.Type]
	if status == 0 {
		status = 502
	}
	err := provider.ClassifyProviderError(status)
	err.Message = provider.SanitizeProviderMessage(wire.Message)
	return err, true
}
