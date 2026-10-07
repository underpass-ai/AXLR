package openrouter

import (
	"net/http"
	"time"
)

type ClientConfig struct {
	APIKey     string
	HTTPClient *http.Client
	// Endpoint is the absolute URL of an OpenAI-compatible chat completions
	// endpoint. Empty uses OpenRouter. A loopback endpoint (localhost,
	// 127.0.0.0/8 or ::1) needs no API key.
	Endpoint string
	// StreamInactivityTimeout limits the wait for headers or the next SSE data event.
	// Zero uses the one-minute default.
	StreamInactivityTimeout time.Duration
	// StreamMaxDuration limits total stream time, even while data keeps arriving.
	// Zero uses the five-minute default.
	StreamMaxDuration time.Duration
}
