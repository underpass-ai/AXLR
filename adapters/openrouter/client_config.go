package openrouter

import "net/http"

type ClientConfig struct {
	APIKey     string
	HTTPClient *http.Client
}
