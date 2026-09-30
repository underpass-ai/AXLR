package openrouter

import (
	"net/http"
	"time"
)

type ClientConfig struct {
	APIKey     string
	HTTPClient *http.Client
	// StreamInactivityTimeout limits the wait for headers or the next SSE data event.
	// Zero uses the one-minute default.
	StreamInactivityTimeout time.Duration
	// StreamMaxDuration limits total stream time, even while data keeps arriving.
	// Zero uses the five-minute default.
	StreamMaxDuration time.Duration
}
