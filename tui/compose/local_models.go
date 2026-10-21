package compose

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/underpass-ai/AXLR/adapters/openrouter"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/adapters/localmodels"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// LocalModels is the composition of settings.json's local_models: the
// /model entries, the context windows, the endpoints the diagnostics trace,
// and the keys that must be redacted and never reach a plugin.
type LocalModels struct {
	Models    []storage.LocalModel
	Keys      map[root.ModelID]string
	Available []domain.AvailableModel
	Windows   localmodels.Windows
	Endpoints []string
	Secrets   []string
	KeyEnvs   map[string]bool
}

func NewLocalModels(settings storage.UserSettings, getenv func(string) string) LocalModels {
	setup := LocalModels{
		Models:  settings.LocalModels,
		Keys:    map[root.ModelID]string{},
		Windows: localmodels.Windows{Local: map[root.ModelID]domain.ContextWindow{}, Cap: domain.ContextWindow(settings.ContextTokens), Prompt: settings.PromptTokens},
		KeyEnvs: map[string]bool{"OPENROUTER_API_KEY": true},
	}
	for _, local := range settings.LocalModels {
		id := root.ModelID(local.ID)
		window := domain.ContextWindow(local.ContextTokens)
		setup.Windows.Local[id] = window
		name := local.Name
		if name == "" {
			name = local.ID
		}
		setup.Available = append(setup.Available, domain.AvailableModel{ID: id, Name: root.Text(name), Context: setup.Windows.ContextWindow(id), SupportsTools: local.SupportsTools(), TextOutput: true})
		setup.Endpoints = append(setup.Endpoints, local.URL)
		if local.APIKeyEnv != "" {
			setup.KeyEnvs[local.APIKeyEnv] = true
			if key := strings.TrimSpace(getenv(local.APIKeyEnv)); key != "" {
				setup.Keys[id] = key
				setup.Secrets = append(setup.Secrets, key)
			}
		}
	}
	return setup
}

// Connect builds one chat client per local model over the binary's HTTP
// client, so their requests pass through the same diagnostics transport.
func (s LocalModels) Connect(httpClient *http.Client) (map[root.ModelID]localmodels.Route, error) {
	routes := make(map[root.ModelID]localmodels.Route, len(s.Models))
	for _, local := range s.Models {
		id := root.ModelID(local.ID)
		client, err := openrouter.New(openrouter.ClientConfig{
			APIKey:                  s.Keys[id],
			Endpoint:                local.ChatCompletionsURL(),
			HTTPClient:              httpClient,
			StreamInactivityTimeout: local.StreamIdle(),
			StreamMaxDuration:       local.StreamMax(),
			Thinking:                local.Thinking,
		})
		if err != nil {
			if local.APIKeyEnv != "" && s.Keys[id] == "" {
				return nil, fmt.Errorf("local model %s: %w (set %s)", local.ID, err, local.APIKeyEnv)
			}
			return nil, fmt.Errorf("local model %s: %w", local.ID, err)
		}
		routes[id] = localmodels.Route{Client: client, Upstream: root.ModelID(local.UpstreamModel()), NoStream: !local.Streams()}
	}
	return routes, nil
}

// PluginGetenv resolves host variables for plugins and refuses the model
// keys: OPENROUTER_API_KEY and every local model's api_key_env.
// TYPESAFE_API_KEY stays available: KMP reads it for its own Jev features.
func (s LocalModels) PluginGetenv(getenv func(string) string, refused *string) func(string) string {
	return func(name string) string {
		if s.KeyEnvs[name] {
			if refused != nil && *refused == "" {
				*refused = name
			}
			return ""
		}
		return getenv(name)
	}
}

// EndpointsByModel is each local model's server.
func (s LocalModels) EndpointsByModel() map[root.ModelID]string {
	out := map[root.ModelID]string{}
	for _, model := range s.Models {
		out[root.ModelID(model.ID)] = model.URL
	}
	return out
}
