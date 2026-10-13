package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
)

// localCandidate is one model a local OpenAI-compatible server serves.
type localCandidate struct {
	URL     string
	Served  string
	Context int
}

func (c localCandidate) describe() string {
	text := c.Served + " · " + c.URL
	if c.Context > 0 {
		text += fmt.Sprintf(" · %dk", c.Context/1000)
	}
	return text
}

// localModel is the local_models entry: local/<name>, the served name when
// the server needs it, and the window the person confirmed.
func (c localCandidate) localModel(window int) storage.LocalModel {
	name := strings.ToLower(path.Base(strings.ReplaceAll(c.Served, "\\", "/")))
	if c.Served == "" {
		// A server that names no model is known by its address.
		if parsed, err := url.Parse(c.URL); err == nil {
			name = parsed.Hostname() + "-" + parsed.Port()
		}
	}
	name = strings.TrimSuffix(name, ".gguf")
	var slug strings.Builder
	dash := false
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' {
			slug.WriteRune(r)
			dash = false
		} else if !dash && slug.Len() > 0 {
			slug.WriteByte('-')
			dash = true
		}
	}
	id := strings.Trim(slug.String(), "-.")
	if len(id) > 64 {
		id = id[:64]
	}
	if id == "" || id == "." {
		id = "model"
	}
	local := storage.LocalModel{ID: "local/" + id, Name: c.Served, URL: c.URL, ContextTokens: window}
	if _, err := root.NewModelID(c.Served); err == nil && c.Served != "" {
		local.Model = c.Served
	}
	if local.Name == "" {
		local.Name = local.ID
	}
	return local
}

// probeLocalServer lists the models of an OpenAI-compatible server at base
// (…/v1). llama.cpp reports its configured window on /props, vLLM as
// max_model_len, and LM Studio or Ollama report none.
func probeLocalServer(ctx context.Context, client *http.Client, base string) []localCandidate {
	parsed, err := url.Parse(base)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil
	}
	var listing struct {
		Data []struct {
			ID            string `json:"id"`
			MaxModelLen   int    `json:"max_model_len"`
			ContextLength int    `json:"context_length"`
			Meta          struct {
				TrainContext int `json:"n_ctx_train"`
			} `json:"meta"`
		} `json:"data"`
	}
	if !getJSON(ctx, client, base+"/models", &listing) {
		return nil
	}
	var props struct {
		Settings struct {
			Context int `json:"n_ctx"`
		} `json:"default_generation_settings"`
	}
	getJSON(ctx, client, strings.TrimSuffix(base, "/v1")+"/props", &props)
	var found []localCandidate
	for _, model := range listing.Data {
		if model.ID == "" || len(found) == 8 {
			continue
		}
		window := props.Settings.Context
		for _, other := range []int{model.MaxModelLen, model.ContextLength, model.Meta.TrainContext} {
			if window == 0 {
				window = other
			}
		}
		found = append(found, localCandidate{URL: base, Served: model.ID, Context: window})
	}
	return found
}

func getJSON(ctx context.Context, client *http.Client, address string, into any) bool {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return false
	}
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return false
	}
	return json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(into) == nil
}
