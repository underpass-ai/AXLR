package storage

// Credentials is credentials.json beside settings.json: the OpenRouter key
// the first-run setup saved, for a person who does not export
// OPENROUTER_API_KEY. The variable wins when both exist. The file is private
// (0600) and is never given to plugins, tools or the model.
type Credentials struct {
	OpenRouterAPIKey string `json:"openrouter_api_key,omitempty"`
	// TypeSafeAPIKey is Jev's key, saved by setup; TYPESAFE_API_KEY wins.
	TypeSafeAPIKey string `json:"typesafe_api_key,omitempty"`
}
