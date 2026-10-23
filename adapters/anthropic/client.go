package anthropic

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

	"github.com/underpass-ai/AXLR/adapters/provider"
	"github.com/underpass-ai/AXLR/domain"
)

// Endpoint is Anthropic's Messages endpoint.
const Endpoint = "https://api.anthropic.com/v1/messages"

// apiVersion is the Messages API version every request names.
const apiVersion = "2023-06-01"

const (
	defaultStreamInactivityTimeout = 5 * time.Minute
	defaultStreamMaxDuration       = 5 * time.Minute
)

type Client struct {
	apiKey                  string
	endpoint                string
	provider                string
	host                    string
	thinking                bool
	maxTokens               int
	http                    http.Client
	streamInactivityTimeout time.Duration
	streamMaxDuration       time.Duration
}

func New(config ClientConfig) (*Client, error) {
	target := Endpoint
	if config.Endpoint != "" {
		if err := provider.ValidateEndpoint(config.Endpoint); err != nil {
			return nil, err
		}
		target = config.Endpoint
	}
	name := provider.EndpointName(target)
	if strings.ContainsAny(config.APIKey, "\r\n") {
		return nil, errors.New(name + " API key is invalid")
	}
	if strings.TrimSpace(config.APIKey) == "" && !provider.IsLoopbackEndpoint(target) {
		return nil, errors.New(name + " API key is required")
	}
	if config.StreamInactivityTimeout < 0 || config.StreamMaxDuration < 0 {
		return nil, errors.New(name + " stream timeouts must not be negative")
	}
	if config.MaxTokens < 0 {
		return nil, errors.New(name + " max_tokens must not be negative")
	}
	inactivity, maximum := config.StreamInactivityTimeout, config.StreamMaxDuration
	if inactivity == 0 {
		inactivity = defaultStreamInactivityTimeout
	}
	if maximum == 0 {
		maximum = defaultStreamMaxDuration
	}
	client := http.Client{}
	if config.HTTPClient != nil {
		client = *config.HTTPClient
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{
		apiKey:                  strings.TrimSpace(config.APIKey),
		endpoint:                target,
		provider:                name,
		host:                    provider.EndpointHost(target),
		thinking:                config.Thinking == nil || *config.Thinking,
		maxTokens:               config.MaxTokens,
		http:                    client,
		streamInactivityTimeout: inactivity,
		streamMaxDuration:       maximum,
	}, nil
}

// Complete asks for a whole reply.
func (c *Client) Complete(ctx context.Context, req domain.CompletionRequest) (domain.CompletionResult, error) {
	if err := ctx.Err(); err != nil {
		return domain.CompletionResult{}, err
	}
	wire, err := mapRequest(req, c.maxTokens, c.thinking)
	if err != nil {
		return domain.CompletionResult{}, err
	}
	body, err := json.Marshal(wire)
	if err != nil {
		return domain.CompletionResult{}, errors.New("could not encode " + c.provider + " request")
	}
	sent := domain.CompletionResult{RequestBytes: len(body), Provider: c.host}
	requestCtx, cancel := context.WithTimeoutCause(ctx, c.streamMaxDuration, provider.NewStreamTimeoutError(c.provider, true))
	defer cancel()
	failure := func(err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var timeout *provider.StreamTimeoutError
		if errors.As(context.Cause(requestCtx), &timeout) {
			return timeout
		}
		return err
	}
	httpReq, err := c.request(requestCtx, body, false)
	if err != nil {
		return domain.CompletionResult{}, err
	}
	httpResp, err := c.http.Do(httpReq)
	if err != nil {
		return sent, failure(&provider.TransportError{Cause: err, Provider: c.provider})
	}
	var closeOnce sync.Once
	closeBody := func() { closeOnce.Do(func() { _ = httpResp.Body.Close() }) }
	stop := context.AfterFunc(requestCtx, closeBody)
	defer func() { stop(); closeBody() }()
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return sent, c.httpError(httpResp)
	}
	data, err := io.ReadAll(io.LimitReader(httpResp.Body, provider.MaxResponseBytes+1))
	if err != nil {
		return sent, failure(&provider.TransportError{Cause: err, Provider: c.provider})
	}
	if len(data) > provider.MaxResponseBytes {
		return sent, errors.New(c.provider + " response exceeds 8 MiB")
	}
	var reply responseDTO
	if err := json.Unmarshal(data, &reply); err != nil {
		return sent, errors.New("malformed " + c.provider + " response")
	}
	if reply.Type == "error" {
		if err, ok := providerError(reply.Error); ok {
			err.Provider = c.provider
			return sent, err.WithoutKey(c.apiKey)
		}
	}
	var rawContent struct {
		Content []json.RawMessage `json:"content"`
	}
	_ = json.Unmarshal(data, &rawContent)
	result, err := mapResponse(reply, rawContent.Content, c.host)
	if err != nil {
		return sent, err
	}
	result.RequestBytes = len(body)
	return result, nil
}

// request builds the HTTP request with Anthropic's headers: the key as
// x-api-key, never a bearer token, and the thinking beta when thinking is on.
func (c *Client) request(ctx context.Context, body []byte, stream bool) (*http.Request, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("could not create " + c.provider + " request")
	}
	if c.apiKey != "" {
		httpReq.Header.Set("x-api-key", c.apiKey)
	}
	httpReq.Header.Set("anthropic-version", apiVersion)
	httpReq.Header.Set("Content-Type", "application/json")
	if c.thinking {
		httpReq.Header.Set("anthropic-beta", thinkingBeta)
	}
	if stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	}
	return httpReq, nil
}

// httpError classifies an HTTP error reply and keeps its message, without
// the client's key.
func (c *Client) httpError(response *http.Response) *provider.ProviderError {
	message, _ := provider.ReadErrorResponse(response.Body)
	err := provider.ClassifyProviderError(response.StatusCode)
	err.Message, err.Provider = message, c.provider
	return err.WithoutKey(c.apiKey)
}
