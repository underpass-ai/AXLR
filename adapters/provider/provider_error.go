package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// DefaultProviderName labels an error whose Provider is empty: OpenRouter,
// the default endpoint. Every other adapter names its endpoint.
const DefaultProviderName = "OpenRouter"

type ProviderError struct {
	StatusCode int
	Category   ProviderErrorCategory
	// Provider names a non-OpenRouter endpoint; empty means OpenRouter.
	Provider string
	// Message is what the provider said about the failure, on one line, at
	// most MaxProviderMessageRunes, with what looks like a key or a token
	// redacted; empty when it said nothing readable. On 12 October 2026 a
	// wrong model id, a context overflow and a malformed request all read
	// "OpenRouter invalid_request (HTTP 400)", while the body said
	// "anthropic/claude-nonexistent-9 is not a valid model ID".
	Message string
}

// MaxProviderMessageRunes bounds the provider's message kept in an error.
const MaxProviderMessageRunes = 300

func (e *ProviderError) Error() string {
	provider := e.Provider
	if provider == "" {
		provider = DefaultProviderName
	}
	if e.Message != "" {
		return fmt.Sprintf("%s %s (HTTP %d): %s", provider, e.Category, e.StatusCode, e.Message)
	}
	return fmt.Sprintf("%s %s (HTTP %d)", provider, e.Category, e.StatusCode)
}

// invalidModel recognises the messages providers send for a model id they
// do not serve: OpenRouter's "… is not a valid model ID" and "No endpoints
// found for …", the OpenAI-compatible servers' "model … does not exist" or
// "… not found", and Anthropic's bare "model: claude-x".
var invalidModel = regexp.MustCompile(`(?i)(not a valid model|no endpoints found|model\b.*\b(does not exist|not found|is not supported)|unknown model|invalid model|^model: )`)

// HintKind is what the person can do about the error, HintNone when the
// error says nothing they can act on.
func (e *ProviderError) HintKind() ProviderHint {
	switch e.Category {
	case CategoryAuthentication:
		return HintCheckKey
	case CategoryInsufficientCredits:
		return HintCredits
	case CategoryRateLimited:
		return HintRetryLater
	case CategoryInvalidRequest:
		if invalidModel.MatchString(e.Message) {
			return HintChooseModel
		}
	}
	return HintNone
}

// Hint is HintKind in English, "" for HintNone.
func (e *ProviderError) Hint() string {
	switch e.HintKind() {
	case HintChooseModel:
		return "choose a model the provider serves with /model"
	case HintCheckKey:
		if e.Provider != "" && e.Provider != DefaultProviderName {
			return "check the key of this endpoint (api_key_env in local_models)"
		}
		return "check OPENROUTER_API_KEY (keys at https://openrouter.ai/keys)"
	case HintCredits:
		if e.Provider != "" && e.Provider != DefaultProviderName {
			return "the account of this endpoint has no credits left"
		}
		return "the account has no credits left; add some at https://openrouter.ai/settings/credits"
	case HintRetryLater:
		return "the provider is rate limiting requests; wait and try again"
	}
	return ""
}

// ClassifyProviderError maps an HTTP status to its category: 529 is
// Anthropic's overloaded_error, a wait like a rate limit.
func ClassifyProviderError(status int) *ProviderError {
	category := CategoryProviderFailure
	switch status {
	case 400, 404, 413, 422:
		category = CategoryInvalidRequest
	case 401, 403:
		category = CategoryAuthentication
	case 402:
		category = CategoryInsufficientCredits
	case 429, 529:
		category = CategoryRateLimited
	}
	return &ProviderError{StatusCode: status, Category: category}
}

// ErrorBody reads the error object a provider sends in a body it answered
// with HTTP 200, in a stream event or a whole reply: its numeric code is the
// status to classify, 502 when it has none, and its message is kept
// sanitized.
func ErrorBody(raw json.RawMessage) (int, string, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, "", false
	}
	var provider struct {
		Code    json.RawMessage `json:"code"`
		Message string          `json:"message"`
	}
	_ = json.Unmarshal(raw, &provider)
	var code int
	_ = json.Unmarshal(provider.Code, &code)
	if code == 0 {
		code = 502
	}
	return code, SanitizeProviderMessage(provider.Message), true
}

// maxErrorBodyBytes bounds the read of an HTTP error body.
const maxErrorBodyBytes = 64 << 10

// ErrorResponseMessage reads the message of an HTTP error body: OpenRouter's,
// Anthropic's and OpenAI's {"error":{"message":…}} or an OpenAI-compatible
// server's top-level {"message":…}. A body that is not JSON gives none: an
// HTML error page is no message.
func ErrorResponseMessage(body io.Reader) string {
	data, _ := io.ReadAll(io.LimitReader(body, maxErrorBodyBytes))
	var reply struct {
		Error   json.RawMessage `json:"error"`
		Message string          `json:"message"`
	}
	if json.Unmarshal(data, &reply) != nil {
		return ""
	}
	if _, message, ok := ErrorBody(reply.Error); ok && message != "" {
		return message
	}
	return SanitizeProviderMessage(reply.Message)
}

// ReadErrorResponse reads the message of an HTTP error body like
// ErrorResponseMessage, and the string code the provider sends with it when
// it does, such as OpenAI's "insufficient_quota" under HTTP 429 or
// Anthropic's error type; "" when there is none.
func ReadErrorResponse(body io.Reader) (message, code string) {
	data, _ := io.ReadAll(io.LimitReader(body, maxErrorBodyBytes))
	var reply struct {
		Error struct {
			Message string          `json:"message"`
			Code    json.RawMessage `json:"code"`
			Type    string          `json:"type"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &reply) != nil {
		return "", ""
	}
	message = SanitizeProviderMessage(reply.Error.Message)
	if message == "" {
		message = SanitizeProviderMessage(reply.Message)
	}
	_ = json.Unmarshal(reply.Error.Code, &code)
	if code == "" {
		code = reply.Error.Type
	}
	return message, code
}

// secretLike matches what looks like a credential in a provider's message:
// a bearer token, an sk- style key or a long unbroken run of key characters.
var secretLike = regexp.MustCompile(`(?i)bearer\s+\S+|\b(sk|pk|rk)-[A-Za-z0-9_\-]{8,}|[A-Za-z0-9+/_\-]{40,}={0,2}`)

// SanitizeProviderMessage puts a provider's message on one line, redacts
// what looks like a credential and bounds it to MaxProviderMessageRunes.
func SanitizeProviderMessage(message string) string {
	message = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == utf8.RuneError {
			return ' '
		}
		return r
	}, message)
	message = strings.Join(strings.Fields(message), " ")
	message = secretLike.ReplaceAllString(message, "[redacted]")
	if utf8.RuneCountInString(message) > MaxProviderMessageRunes {
		runes := []rune(message)
		message = strings.TrimSpace(string(runes[:MaxProviderMessageRunes-1])) + "…"
	}
	return message
}

// WithoutKey removes the client's own key from the error's message, in case
// the provider echoed the request's headers.
func (e *ProviderError) WithoutKey(key string) *ProviderError {
	if key != "" {
		e.Message = strings.ReplaceAll(e.Message, key, "[redacted]")
	}
	return e
}
