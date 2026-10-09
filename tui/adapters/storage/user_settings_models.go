package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode"

	root "github.com/underpass-ai/AXLR/domain"
)

// Per-model request options. models maps an exact model id to fields
// OpenRouter reads for that model: provider (routing preferences),
// reasoning and max_tokens. Without an entry a request carries none of
// them. Seen on 8 October 2026: z-ai/glm-5.3-flash, with no provider
// preference, was routed to OpenInference at $3.109 per million output
// tokens, a 10-second first byte and 19 tokens per second, while
// InferenceNet served it at $0.50, 1 to 2 seconds and up to 177 tokens per
// second; output was 75 % of that session's cost. An entry is sent as
// written once every key and value is known, so a misspelt key stops the
// launch instead of leaving the routing as it was.

// ModelOptions is one validated entry of models. Provider and Reasoning
// are JSON objects or empty; MaxTokens is zero when absent.
type ModelOptions struct {
	Provider  json.RawMessage
	Reasoning json.RawMessage
	MaxTokens int
}

const (
	maxModelOptions     = 64
	maxCompletionTokens = 1_000_000
	maxProviderSlugs    = 64
)

var (
	providerKeys = []string{"order", "allow_fallbacks", "require_parameters", "data_collection", "zdr", "only", "ignore", "quantizations", "sort", "preferred_min_throughput", "preferred_max_latency", "max_price"}
	// providerSorts is what sort accepts. OpenRouter does not document the
	// metric behind price; docs/console.md says why throughput is the lever.
	providerSorts    = []string{"price", "throughput", "latency"}
	quantizations    = []string{"int4", "int8", "fp4", "mxfp4", "nvfp4", "fp6", "fp8", "mxfp8", "fp16", "bf16", "fp32", "unknown"}
	reasoningEfforts = []string{"max", "xhigh", "high", "medium", "low", "minimal", "none"}
)

// ModelRequestOptions is models by model id. A loaded file has passed
// Validate, so no entry is left out.
func (s UserSettings) ModelRequestOptions() map[string]ModelOptions {
	options := make(map[string]ModelOptions, len(s.Models))
	for id, raw := range s.Models {
		if parsed, err := parseModelOptions(id, raw); err == nil {
			options[id] = parsed
		}
	}
	return options
}

// validateModels checks every entry in id order, so the first error is
// stable. local holds the local_models ids: their servers receive none of
// these fields, so an entry for one is refused rather than ignored.
func (s UserSettings) validateModels(local map[string]struct{}) error {
	if len(s.Models) > maxModelOptions {
		return fmt.Errorf("settings models lists more than %d models", maxModelOptions)
	}
	for _, id := range slices.Sorted(maps.Keys(s.Models)) {
		if _, err := root.NewModelID(id); err != nil {
			return errors.New("settings models has an invalid model id")
		}
		if _, ok := local[id]; ok {
			return fmt.Errorf("settings models %s is a local model; provider, reasoning and max_tokens are sent only to OpenRouter", id)
		}
		if _, err := parseModelOptions(id, s.Models[id]); err != nil {
			return err
		}
	}
	return nil
}

func parseModelOptions(id string, raw json.RawMessage) (ModelOptions, error) {
	at := optionPath{model: id}
	fields, err := at.object(raw, "provider", "reasoning", "max_tokens")
	if err != nil {
		return ModelOptions{}, err
	}
	var options ModelOptions
	if value, ok := fields["provider"]; ok {
		if err := at.child("provider").provider(value); err != nil {
			return ModelOptions{}, err
		}
		options.Provider = value
	}
	if value, ok := fields["reasoning"]; ok {
		if err := at.child("reasoning").reasoning(value); err != nil {
			return ModelOptions{}, err
		}
		options.Reasoning = value
	}
	if value, ok := fields["max_tokens"]; ok {
		if options.MaxTokens, err = at.child("max_tokens").tokens(value); err != nil {
			return ModelOptions{}, err
		}
	}
	return options, nil
}

// optionPath names a value in errors: settings models <id> provider.sort.
type optionPath struct{ model, name string }

func (p optionPath) String() string {
	if p.name == "" {
		return "settings models " + p.model
	}
	return "settings models " + p.model + " " + p.name
}

func (p optionPath) child(key string) optionPath {
	if p.name == "" {
		return optionPath{model: p.model, name: key}
	}
	return optionPath{model: p.model, name: p.name + "." + key}
}

// object decodes a JSON object whose keys are all known and whose values
// are not null.
func (p optionPath) object(raw json.RawMessage, known ...string) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return nil, fmt.Errorf("%s must be a JSON object", p)
	}
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		if !slices.Contains(known, key) {
			return nil, fmt.Errorf("%s has unknown key %q", p, key)
		}
		if bytes.Equal(bytes.TrimSpace(fields[key]), []byte("null")) {
			return nil, fmt.Errorf("%s must not be null", p.child(key))
		}
	}
	return fields, nil
}

func (p optionPath) provider(raw json.RawMessage) error {
	fields, err := p.object(raw, providerKeys...)
	if err != nil {
		return err
	}
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		value, at := fields[key], p.child(key)
		switch key {
		case "order", "only", "ignore":
			err = at.slugs(value)
		case "allow_fallbacks", "require_parameters", "zdr":
			err = at.boolean(value)
		case "data_collection":
			err = at.oneOf(value, "allow", "deny")
		case "quantizations":
			err = at.listOf(value, quantizations)
		case "sort":
			err = at.sort(value)
		case "preferred_min_throughput", "preferred_max_latency":
			err = at.nonNegative(value)
		case "max_price":
			err = at.prices(value)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// prices is max_price: dollars (per million tokens for prompt and
// completion), each a ceiling an endpoint must not exceed.
func (p optionPath) prices(raw json.RawMessage) error {
	fields, err := p.object(raw, "prompt", "completion", "request", "image")
	if err != nil {
		return err
	}
	for _, kind := range slices.Sorted(maps.Keys(fields)) {
		if err := p.child(kind).nonNegative(fields[kind]); err != nil {
			return err
		}
	}
	return nil
}

func (p optionPath) reasoning(raw json.RawMessage) error {
	fields, err := p.object(raw, "effort", "max_tokens", "exclude", "enabled")
	if err != nil {
		return err
	}
	if _, effort := fields["effort"]; effort {
		if _, budget := fields["max_tokens"]; budget {
			return fmt.Errorf("%s sets both effort and max_tokens; OpenRouter takes one of them", p)
		}
	}
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		value, at := fields[key], p.child(key)
		switch key {
		case "effort":
			err = at.oneOf(value, reasoningEfforts...)
		case "max_tokens":
			_, err = at.tokens(value)
		case "exclude", "enabled":
			err = at.boolean(value)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// sort is a strategy, or an object whose by is one; partition is passed
// through as a name, since its values are OpenRouter's to define.
func (p optionPath) sort(raw json.RawMessage) error {
	if trimmed := bytes.TrimSpace(raw); len(trimmed) == 0 || trimmed[0] != '{' {
		if err := p.oneOf(raw, providerSorts...); err != nil {
			return fmt.Errorf("%w, or an object with by and partition", err)
		}
		return nil
	}
	fields, err := p.object(raw, "by", "partition")
	if err != nil {
		return err
	}
	by, ok := fields["by"]
	if !ok {
		return fmt.Errorf("%s needs by", p)
	}
	if err := p.child("by").oneOf(by, providerSorts...); err != nil {
		return err
	}
	if partition, ok := fields["partition"]; ok {
		var value string
		if json.Unmarshal(partition, &value) != nil || !plainName(value) {
			return fmt.Errorf("%s must be a name", p.child("partition"))
		}
	}
	return nil
}

func (p optionPath) boolean(raw json.RawMessage) error {
	var value bool
	if json.Unmarshal(raw, &value) != nil {
		return fmt.Errorf("%s must be true or false", p)
	}
	return nil
}

func (p optionPath) oneOf(raw json.RawMessage, allowed ...string) error {
	var value string
	if json.Unmarshal(raw, &value) != nil || !slices.Contains(allowed, value) {
		return fmt.Errorf("%s must be %s", p, alternatives(allowed))
	}
	return nil
}

func (p optionPath) listOf(raw json.RawMessage, allowed []string) error {
	var values []string
	if json.Unmarshal(raw, &values) != nil || len(values) == 0 {
		return fmt.Errorf("%s must be a non-empty list", p)
	}
	for _, value := range values {
		if !slices.Contains(allowed, value) {
			return fmt.Errorf("%s accepts only %s", p, strings.Join(allowed, ", "))
		}
	}
	return nil
}

// slugs is a list of provider slugs, as openrouter.ai lists a model's
// endpoints. An empty list is refused: it orders nothing, and in only it
// would refuse every provider.
func (p optionPath) slugs(raw json.RawMessage) error {
	var values []string
	if json.Unmarshal(raw, &values) != nil || len(values) == 0 || len(values) > maxProviderSlugs {
		return fmt.Errorf("%s must be a list of 1 to %d provider slugs", p, maxProviderSlugs)
	}
	for _, value := range values {
		if !plainName(value) {
			return fmt.Errorf("%s has an empty or invalid provider slug", p)
		}
	}
	return nil
}

func (p optionPath) nonNegative(raw json.RawMessage) error {
	var value float64
	if json.Unmarshal(raw, &value) != nil || value < 0 {
		return fmt.Errorf("%s must be a number of at least 0", p)
	}
	return nil
}

func (p optionPath) tokens(raw json.RawMessage) (int, error) {
	var value int
	if json.Unmarshal(raw, &value) != nil || value < 1 || value > maxCompletionTokens {
		return 0, fmt.Errorf("%s must be a whole number between 1 and %d", p, maxCompletionTokens)
	}
	return value, nil
}

// plainName is non-empty, at most 200 bytes, without surrounding space or
// control characters.
func plainName(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || len(value) > 200 {
		return false
	}
	return !strings.ContainsFunc(value, unicode.IsControl)
}

// alternatives reads "a, b or c" for two values or more.
func alternatives(values []string) string {
	return strings.Join(values[:len(values)-1], ", ") + " or " + values[len(values)-1]
}
