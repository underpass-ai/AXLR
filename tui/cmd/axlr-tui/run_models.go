package main

import (
	"github.com/underpass-ai/AXLR/adapters/openrouter"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
)

// openRouterModelOptions is settings.json's models for the OpenRouter
// client, which adds them to every request naming the model, whether the
// session's, a plan's, a worker's or the reviewer's. Local servers have
// their own clients and receive none.
func openRouterModelOptions(settings storage.UserSettings) map[string]openrouter.ModelOptions {
	configured := settings.ModelRequestOptions()
	options := make(map[string]openrouter.ModelOptions, len(configured))
	for id, entry := range configured {
		options[id] = openrouter.ModelOptions{Provider: entry.Provider, Reasoning: entry.Reasoning, MaxTokens: entry.MaxTokens}
	}
	return options
}
