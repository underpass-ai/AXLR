package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/underpass-ai/AXLR/domain"
)

const endpoint = "https://openrouter.ai/api/v1/chat/completions"
const maxResponseBytes = 8 * 1024 * 1024

type Client struct {
	apiKey                  string
	endpoint                string
	provider                string
	thinking                *bool
	models                  map[string]ModelOptions
	http                    http.Client
	streamInactivityTimeout time.Duration
	streamMaxDuration       time.Duration
}

const defaultStreamInactivityTimeout = time.Minute
const defaultStreamMaxDuration = 5 * time.Minute

func New(config ClientConfig) (*Client, error) {
	target, provider := endpoint, providerName
	if config.Endpoint != "" {
		if err := validateEndpoint(config.Endpoint); err != nil {
			return nil, err
		}
		target, provider = config.Endpoint, endpointName(config.Endpoint)
	}
	if strings.ContainsAny(config.APIKey, "\r\n") {
		return nil, errors.New(provider + " API key is invalid")
	}
	if strings.TrimSpace(config.APIKey) == "" && (config.Endpoint == "" || !IsLoopbackEndpoint(config.Endpoint)) {
		return nil, errors.New(provider + " API key is required")
	}
	if config.StreamInactivityTimeout < 0 {
		return nil, errors.New("OpenRouter stream inactivity timeout must not be negative")
	}
	if config.StreamMaxDuration < 0 {
		return nil, errors.New("OpenRouter stream maximum duration must not be negative")
	}
	streamInactivityTimeout := config.StreamInactivityTimeout
	if streamInactivityTimeout == 0 {
		streamInactivityTimeout = defaultStreamInactivityTimeout
	}
	streamMaxDuration := config.StreamMaxDuration
	if streamMaxDuration == 0 {
		streamMaxDuration = defaultStreamMaxDuration
	}
	models, err := copyModelOptions(config.Models)
	if err != nil {
		return nil, err
	}
	client := http.Client{}
	if config.HTTPClient != nil {
		client = *config.HTTPClient
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{apiKey: strings.TrimSpace(config.APIKey), endpoint: target, provider: provider, thinking: config.Thinking, models: models, http: client, streamInactivityTimeout: streamInactivityTimeout, streamMaxDuration: streamMaxDuration}, nil
}

func (c *Client) Complete(ctx context.Context, req domain.CompletionRequest) (domain.CompletionResult, error) {
	if err := ctx.Err(); err != nil {
		return domain.CompletionResult{}, err
	}
	wire, err := mapRequest(req)
	if err != nil {
		return domain.CompletionResult{}, err
	}
	c.template(&wire)
	body, err := json.Marshal(c.body(wire))
	if err != nil {
		return domain.CompletionResult{}, errors.New("could not encode OpenRouter request")
	}
	// A whole reply has no events to time, so the stream's maximum duration
	// bounds the request and its body: a server that accepts the request and
	// never answers does not hold the turn until the person cancels.
	requestCtx, cancel := context.WithTimeoutCause(ctx, c.streamMaxDuration, &StreamTimeoutError{maximumDuration: true, provider: c.provider})
	defer cancel()
	failure := func(err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var timeout *StreamTimeoutError
		if errors.As(context.Cause(requestCtx), &timeout) {
			return timeout
		}
		return err
	}
	httpReq, err := http.NewRequestWithContext(requestCtx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return domain.CompletionResult{}, errors.New("could not create OpenRouter request")
	}
	c.authorize(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")
	httpResp, err := c.http.Do(httpReq)
	if err != nil {
		return domain.CompletionResult{}, failure(&TransportError{Cause: err, Provider: c.provider})
	}
	var closeOnce sync.Once
	closeBody := func() { closeOnce.Do(func() { _ = httpResp.Body.Close() }) }
	stop := context.AfterFunc(requestCtx, closeBody)
	defer func() { stop(); closeBody() }()
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return domain.CompletionResult{}, c.providerError(httpResp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(httpResp.Body, maxResponseBytes+1))
	if err != nil {
		return domain.CompletionResult{}, failure(&TransportError{Cause: err, Provider: c.provider})
	}
	if len(data) > maxResponseBytes {
		return domain.CompletionResult{}, errors.New("OpenRouter response exceeds 8 MiB")
	}
	var reply responseDTO
	if err := json.Unmarshal(data, &reply); err != nil {
		return domain.CompletionResult{}, errors.New("malformed OpenRouter response")
	}
	if status, ok := errorBodyStatus(reply.Error); ok {
		return domain.CompletionResult{}, c.providerError(status)
	}
	return mapResponse(reply)
}

// body is what the request encodes: for OpenRouter, with the options
// configured for the model and marked for the prompt cache when the model
// supports it; the wire request otherwise. Complete and Stream both encode
// through it.
func (c *Client) body(wire requestDTO) any {
	openRouter := c.endpoint == endpoint
	if openRouter {
		wire.ModelOptions = c.models[wire.Model]
	}
	return promptCacheBody(wire, openRouter)
}

// authorize sets the bearer header only when a key is configured: a loopback
// server without authentication receives no Authorization header at all.
func (c *Client) authorize(req *http.Request) {
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
}

func (c *Client) providerError(status int) *ProviderError {
	err := classifyProviderError(status)
	if c.provider != providerName {
		err.Provider = c.provider
	}
	return err
}

// template asks the chat template to turn thinking off when configured.
func (c *Client) template(wire *requestDTO) {
	if c.thinking != nil && !*c.thinking {
		wire.ChatTemplateKwargs = map[string]any{"enable_thinking": false}
	}
}
