package anthropic

import (
	"net/http"
	"time"
)

type ClientConfig struct {
	// APIKey is sent as x-api-key; required unless Endpoint is loopback.
	APIKey     string
	HTTPClient *http.Client
	// Endpoint is the absolute URL of a Messages endpoint, such as
	// https://api.anthropic.com/v1/messages. Empty uses Anthropic's.
	Endpoint string
	// Thinking false sends no thinking configuration, leaving the model's
	// own default (off before Claude 4.6, on from Claude Opus 5); nil or true
	// asks for adaptive thinking with readable summaries, and lets the API
	// drop a thinking block the conversation no longer matches instead of
	// refusing the request (AXLR cuts its history at the context cap).
	Thinking *bool
	// MaxTokens caps the reply; the Messages API requires it. Zero uses
	// DefaultMaxTokens.
	MaxTokens int
	// StreamInactivityTimeout limits the wait for headers or the next SSE
	// event. Zero uses the five-minute default.
	StreamInactivityTimeout time.Duration
	// StreamMaxDuration limits total stream time, even while data keeps
	// arriving, and the whole request of Complete. Zero uses the
	// five-minute default.
	StreamMaxDuration time.Duration
}
