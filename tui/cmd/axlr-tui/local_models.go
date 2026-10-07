package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/underpass-ai/AXLR/adapters/openrouter"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/adapters/localmodels"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/adapters/typesafe"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// localModelSetup is the composition of settings.json's local_models: the
// /model entries, the context windows, the endpoints the diagnostics trace,
// and the keys that must be redacted and never reach a plugin.
type localModelSetup struct {
	models    []storage.LocalModel
	keys      map[root.ModelID]string
	available []domain.AvailableModel
	windows   localmodels.Windows
	endpoints []string
	secrets   []string
	keyEnvs   map[string]bool
}

func newLocalModelSetup(settings storage.UserSettings, getenv func(string) string) localModelSetup {
	setup := localModelSetup{
		models:  settings.LocalModels,
		keys:    map[root.ModelID]string{},
		windows: localmodels.Windows{Local: map[root.ModelID]domain.ContextWindow{}, Cap: domain.ContextWindow(settings.ContextTokens)},
		keyEnvs: map[string]bool{"OPENROUTER_API_KEY": true},
	}
	for _, local := range settings.LocalModels {
		id := root.ModelID(local.ID)
		window := domain.ContextWindow(local.ContextTokens)
		setup.windows.Local[id] = window
		name := local.Name
		if name == "" {
			name = local.ID
		}
		setup.available = append(setup.available, domain.AvailableModel{ID: id, Name: root.Text(name), Context: setup.windows.ContextWindow(id), SupportsTools: local.SupportsTools(), TextOutput: true})
		setup.endpoints = append(setup.endpoints, local.URL)
		if local.APIKeyEnv != "" {
			setup.keyEnvs[local.APIKeyEnv] = true
			if key := strings.TrimSpace(getenv(local.APIKeyEnv)); key != "" {
				setup.keys[id] = key
				setup.secrets = append(setup.secrets, key)
			}
		}
	}
	return setup
}

// connect builds one chat client per local model over the console's HTTP
// client, so their requests pass through the same diagnostics transport.
func (s localModelSetup) connect(httpClient *http.Client) (map[root.ModelID]localmodels.Route, error) {
	routes := make(map[root.ModelID]localmodels.Route, len(s.models))
	for _, local := range s.models {
		id := root.ModelID(local.ID)
		client, err := openrouter.New(openrouter.ClientConfig{
			APIKey:                  s.keys[id],
			Endpoint:                local.ChatCompletionsURL(),
			HTTPClient:              httpClient,
			StreamInactivityTimeout: local.StreamIdle(),
			StreamMaxDuration:       local.StreamMax(),
		})
		if err != nil {
			if local.APIKeyEnv != "" && s.keys[id] == "" {
				return nil, fmt.Errorf("local model %s: %w (set %s)", local.ID, err, local.APIKeyEnv)
			}
			return nil, fmt.Errorf("local model %s: %w", local.ID, err)
		}
		routes[id] = localmodels.Route{Client: client, Upstream: root.ModelID(local.UpstreamModel()), NoStream: !local.Streams()}
	}
	return routes, nil
}

// pluginGetenv resolves host variables for plugins and refuses the model
// keys: OPENROUTER_API_KEY and every local model's api_key_env.
// TYPESAFE_API_KEY stays available: KMP reads it for its own Jev features.
func (s localModelSetup) pluginGetenv(getenv func(string) string, refused *string) func(string) string {
	return func(name string) string {
		if s.keyEnvs[name] {
			if refused != nil && *refused == "" {
				*refused = name
			}
			return ""
		}
		return getenv(name)
	}
}

// jevJudge builds the TypeSafe Jev judge when settings enable it; nil keeps
// Jev off and sends nothing to TypeSafe.
func jevJudge(settings *storage.JevSettings, getenv func(string) string) (*application.Judge, error) {
	if !settings.Enabled() {
		return nil, nil
	}
	client, err := typesafe.New(typesafe.Config{APIKey: getenv("TYPESAFE_API_KEY"), Model: settings.Model, Timeout: time.Duration(settings.TimeoutMS) * time.Millisecond})
	if err != nil {
		return nil, fmt.Errorf("jev: %w", err)
	}
	return &application.Judge{Port: client, Tool: settings.Tool, FinalCheck: settings.FinalCheck, Threshold: settings.FinalThreshold}, nil
}

// compactProfile decides the ceremony profile per session model: always or
// never when settings say so, otherwise compact for a known window of at
// most storage.CompactWindowTokens.
func compactProfile(profile string, windows application.ModelContextWindowPort) func(root.ModelID) bool {
	switch profile {
	case "compact":
		return func(root.ModelID) bool { return true }
	case "standard":
		return nil
	}
	return func(model root.ModelID) bool {
		window := windows.ContextWindow(model)
		return window > 0 && window.Tokens() <= storage.CompactWindowTokens
	}
}
