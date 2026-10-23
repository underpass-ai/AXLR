package openai

import (
	"encoding/json"

	"github.com/underpass-ai/AXLR/adapters/provider"
)

// errorDTO is the error object OpenAI sends in a body, a failed response or
// a stream event: {"code":"insufficient_quota","message":"…","type":…}.
type errorDTO struct {
	Code    json.RawMessage `json:"code"`
	Message string          `json:"message"`
	Type    string          `json:"type"`
}

// errorStatuses are the HTTP statuses OpenAI's string codes stand for, for
// a stream event that carries no status, and the one code that says more
// than its status: insufficient_quota arrives under HTTP 429 but means the
// account has no credits, not that it is sending too fast.
var errorStatuses = map[string]int{
	"insufficient_quota":         402,
	"billing_hard_limit_reached": 402,
	"rate_limit_exceeded":        429,
	"invalid_api_key":            401,
	"authentication_error":       401,
	"model_not_found":            404,
	"invalid_request_error":      400,
	"context_length_exceeded":    400,
	"server_error":               500,
}

// classify maps an HTTP status and a string code to the error category.
func classify(status int, code string) *provider.ProviderError {
	if byCode, known := errorStatuses[code]; known && (status == 0 || byCode == 402) {
		status = byCode
	}
	if status == 0 {
		status = 502
	}
	return provider.ClassifyProviderError(status)
}

// providerError reads an error object; false when raw holds none.
func providerError(raw json.RawMessage) (*provider.ProviderError, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, false
	}
	var wire errorDTO
	_ = json.Unmarshal(raw, &wire)
	var code string
	_ = json.Unmarshal(wire.Code, &code)
	if code == "" {
		code = wire.Type
	}
	err := classify(0, code)
	err.Message = provider.SanitizeProviderMessage(wire.Message)
	return err, true
}
