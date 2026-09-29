package openrouter

import (
	"net/http"
	"time"
)

type ClientConfig struct {
	APIKey                  string
	HTTPClient              *http.Client
	StreamInactivityTimeout time.Duration
}
