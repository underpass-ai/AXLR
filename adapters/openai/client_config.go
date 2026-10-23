package openai

import (
	"net/http"
	"time"
)

type ClientConfig struct {
	// APIKey is sent as a bearer token; required unless Endpoint is loopback.
	APIKey     string
	HTTPClient *http.Client
	// Endpoint is the absolute URL of a Responses endpoint, such as
	// https://api.openai.com/v1/responses. Empty uses OpenAI's.
	Endpoint string
	// Thinking false asks for no reasoning summaries and no encrypted
	// reasoning, for a model without reasoning, which refuses the reasoning
	// parameter; nil or true keeps the model's reasoning across tool calls.
	Thinking *bool
	// StreamInactivityTimeout limits the wait for headers or the next SSE
	// event. Zero uses the five-minute default.
	StreamInactivityTimeout time.Duration
	// StreamMaxDuration limits total stream time, even while data keeps
	// arriving, and the whole request of Complete. Zero uses the
	// five-minute default.
	StreamMaxDuration time.Duration
}
