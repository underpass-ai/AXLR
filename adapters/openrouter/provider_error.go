package openrouter

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

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
		provider = providerName
	}
	if e.Message != "" {
		return fmt.Sprintf("%s %s (HTTP %d): %s", provider, e.Category, e.StatusCode, e.Message)
	}
	return fmt.Sprintf("%s %s (HTTP %d)", provider, e.Category, e.StatusCode)
}

// ProviderHint names what the person can do about a provider error; the
// console words it in its language.
type ProviderHint string

const (
	HintNone ProviderHint = ""
	// HintChooseModel: the model id is not one the provider serves.
	HintChooseModel ProviderHint = "choose_model"
	// HintCheckKey: the provider refused the key.
	HintCheckKey ProviderHint = "check_key"
	// HintCredits: the account has no credits left.
	HintCredits ProviderHint = "credits"
	// HintRetryLater: the provider is rate limiting the requests.
	HintRetryLater ProviderHint = "retry_later"
)

// invalidModel recognises the messages providers send for a model id they
// do not serve: OpenRouter's "… is not a valid model ID" and "No endpoints
// found for …", and the OpenAI-compatible servers' "model … does not exist"
// or "… not found".
var invalidModel = regexp.MustCompile(`(?i)(not a valid model|no endpoints found|model\b.*\b(does not exist|not found|is not supported)|unknown model|invalid model)`)

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
		if e.Provider != "" && e.Provider != providerName {
			return "check the key of this endpoint (api_key_env in local_models)"
		}
		return "check OPENROUTER_API_KEY (keys at https://openrouter.ai/keys)"
	case HintCredits:
		return "the account has no credits left; add some at https://openrouter.ai/settings/credits"
	case HintRetryLater:
		return "the provider is rate limiting requests; wait and try again"
	}
	return ""
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

// errorBody reads the error object a provider sends in a body it answered
// with HTTP 200, in a stream event or a whole reply: its code is the status
// to classify, 502 when it has none, and its message is kept sanitized.
func errorBody(raw json.RawMessage) (int, string, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, "", false
	}
	var provider struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(raw, &provider)
	if provider.Code == 0 {
		provider.Code = 502
	}
	return provider.Code, sanitizeProviderMessage(provider.Message), true
}

// maxErrorBodyBytes bounds the read of an HTTP error body.
const maxErrorBodyBytes = 64 << 10

// errorResponseMessage reads the message of an HTTP error body: OpenRouter's
// {"error":{"message":…}} or an OpenAI-compatible server's top-level
// {"message":…}. A body that is not JSON gives none: an HTML error page is
// no message.
func errorResponseMessage(body io.Reader) string {
	data, _ := io.ReadAll(io.LimitReader(body, maxErrorBodyBytes))
	var reply struct {
		Error   json.RawMessage `json:"error"`
		Message string          `json:"message"`
	}
	if json.Unmarshal(data, &reply) != nil {
		return ""
	}
	if _, message, ok := errorBody(reply.Error); ok && message != "" {
		return message
	}
	return sanitizeProviderMessage(reply.Message)
}

// secretLike matches what looks like a credential in a provider's message:
// a bearer token, an sk- style key or a long unbroken run of key characters.
var secretLike = regexp.MustCompile(`(?i)bearer\s+\S+|\b(sk|pk|rk)-[A-Za-z0-9_\-]{8,}|[A-Za-z0-9+/_\-]{40,}={0,2}`)

// sanitizeProviderMessage puts a provider's message on one line, redacts
// what looks like a credential and bounds it to MaxProviderMessageRunes.
func sanitizeProviderMessage(message string) string {
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

// withoutKey removes the client's own key from the error's message, in case
// the provider echoed the request's headers.
func (e *ProviderError) withoutKey(key string) *ProviderError {
	if key != "" {
		e.Message = strings.ReplaceAll(e.Message, key, "[redacted]")
	}
	return e
}
