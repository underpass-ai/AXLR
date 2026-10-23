package storage

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// LocalModel is one entry of local_models. ID is what the session stores and
// /model shows (a "local/" prefix keeps it apart from OpenRouter ids); URL is
// the server's OpenAI base URL, such as http://127.0.0.1:8080/v1, to which
// /chat/completions is appended; Model is the name sent to the server
// (default ID; vLLM needs its served model name, llama.cpp ignores it);
// APIKeyEnv names the environment variable holding the key, required unless
// the URL is a loopback address; ContextTokens is the window the console
// respects, which may be smaller than the server's; Tools false hides a model
// whose server cannot return native tool calls, since AXLR needs them. API
// chooses the protocol: "chat" (default) is OpenAI's chat completions, which
// every local server speaks, "anthropic" the Anthropic Messages API and
// "responses" the OpenAI Responses API, spoken natively so reasoning, prompt
// caching and usage no longer depend on OpenRouter's translation.
type LocalModel struct {
	ID                string `json:"id"`
	Name              string `json:"name,omitempty"`
	URL               string `json:"url"`
	Model             string `json:"model,omitempty"`
	API               string `json:"api,omitempty"`
	APIKeyEnv         string `json:"api_key_env,omitempty"`
	ContextTokens     int    `json:"context_tokens"`
	Tools             *bool  `json:"tools,omitempty"`
	StreamIdleSeconds int    `json:"stream_idle_seconds,omitempty"`
	StreamMaxMinutes  int    `json:"stream_max_minutes,omitempty"`
	// Stream false asks the server for whole replies instead of a stream.
	Stream *bool `json:"stream,omitempty"`
	// Thinking false turns the model's thinking off through the chat
	// template (enable_thinking); workers want short replies.
	Thinking *bool `json:"thinking,omitempty"`
}

// Streams reports whether requests to this server are streamed (default).
func (m LocalModel) Streams() bool { return m.Stream == nil || *m.Stream }

// Defaults for a local model's stream: a cold prefill of a long prompt on a
// local GPU takes minutes before the first token, and decoding runs at a few
// tokens per second.
const (
	DefaultLocalStreamIdleSeconds = 600
	DefaultLocalStreamMaxMinutes  = 60
	maxLocalModels                = 16
	maxContextTokens              = 16 << 20
)

// SupportsTools reports whether the server returns native tool calls.
func (m LocalModel) SupportsTools() bool { return m.Tools == nil || *m.Tools }

// UpstreamModel is the model name sent to the server.
func (m LocalModel) UpstreamModel() string {
	if m.Model != "" {
		return m.Model
	}
	return m.ID
}

// The protocols a local_models entry may name.
const (
	APIChat      = "chat"
	APIAnthropic = "anthropic"
	APIResponses = "responses"
)

// Protocol is the API the entry speaks, APIChat when it names none.
func (m LocalModel) Protocol() string {
	if m.API == "" {
		return APIChat
	}
	return m.API
}

// ChatCompletionsURL is the endpoint the console posts to for APIChat.
func (m LocalModel) ChatCompletionsURL() string {
	return strings.TrimRight(m.URL, "/") + "/chat/completions"
}

// EndpointURL is the endpoint the console posts to: <url>/chat/completions
// for chat, <url>/v1/messages for anthropic (<url>/messages when the url
// already ends in /v1) and <url>/responses for responses.
func (m LocalModel) EndpointURL() string {
	base := strings.TrimRight(m.URL, "/")
	switch m.Protocol() {
	case APIAnthropic:
		if strings.HasSuffix(base, "/v1") {
			return base + "/messages"
		}
		return base + "/v1/messages"
	case APIResponses:
		return base + "/responses"
	}
	return m.ChatCompletionsURL()
}

// StreamIdle and StreamMax are the stream limits with defaults applied.
func (m LocalModel) StreamIdle() time.Duration {
	if m.StreamIdleSeconds > 0 {
		return time.Duration(m.StreamIdleSeconds) * time.Second
	}
	return DefaultLocalStreamIdleSeconds * time.Second
}

func (m LocalModel) StreamMax() time.Duration {
	if m.StreamMaxMinutes > 0 {
		return time.Duration(m.StreamMaxMinutes) * time.Minute
	}
	return DefaultLocalStreamMaxMinutes * time.Minute
}

func (m LocalModel) validate() error {
	if _, err := root.NewModelID(m.ID); err != nil {
		return errors.New("settings local_models entry has an invalid id")
	}
	if !utf8.ValidString(m.Name) || len(m.Name) > 200 || strings.ContainsAny(m.Name, "\r\n") {
		return fmt.Errorf("settings local_models %s has an invalid name", m.ID)
	}
	parsed, err := url.Parse(m.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("settings local_models %s url must be an absolute http(s) URL without credentials, query or fragment", m.ID)
	}
	if m.Model != "" {
		if _, err := root.NewModelID(m.Model); err != nil {
			return fmt.Errorf("settings local_models %s has an invalid model", m.ID)
		}
	}
	switch m.API {
	case "", APIChat, APIAnthropic, APIResponses:
	default:
		return fmt.Errorf("settings local_models %s api must be chat, anthropic or responses", m.ID)
	}
	if m.APIKeyEnv != "" && !mcpEnvironmentName.MatchString(m.APIKeyEnv) {
		return fmt.Errorf("settings local_models %s api_key_env must be an environment variable name", m.ID)
	}
	if m.ContextTokens < domain.MinimumContextWindow || m.ContextTokens > maxContextTokens {
		return fmt.Errorf("settings local_models %s context_tokens must be between %d and %d", m.ID, domain.MinimumContextWindow, maxContextTokens)
	}
	if m.StreamIdleSeconds < 0 || m.StreamIdleSeconds > 3600 {
		return fmt.Errorf("settings local_models %s stream_idle_seconds must be between 1 and 3600", m.ID)
	}
	if m.StreamMaxMinutes < 0 || m.StreamMaxMinutes > 720 {
		return fmt.Errorf("settings local_models %s stream_max_minutes must be between 1 and 720", m.ID)
	}
	return nil
}
