package openai

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

// Endpoint is OpenAI's Responses endpoint.
const Endpoint = "https://api.openai.com/v1/responses"

const (
	defaultStreamInactivityTimeout = 5 * time.Minute
	defaultStreamMaxDuration       = 5 * time.Minute
)

type Client struct {
	apiKey                  string
	endpoint                string
	provider                string
	host                    string
	reasoning               bool
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
		reasoning:               config.Thinking == nil || *config.Thinking,
		http:                    client,
		streamInactivityTimeout: inactivity,
		streamMaxDuration:       maximum,
	}, nil
}

// Complete asks for a whole response.
func (c *Client) Complete(ctx context.Context, req domain.CompletionRequest) (domain.CompletionResult, error) {
	if err := ctx.Err(); err != nil {
		return domain.CompletionResult{}, err
	}
	wire, err := mapRequest(req, c.reasoning)
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
	result, err := mapResponse(reply, c.host)
	if err != nil {
		var failed *provider.ProviderError
		if errors.As(err, &failed) {
			failed.Provider = c.provider
			failed.WithoutKey(c.apiKey)
		}
		return sent, err
	}
	result.RequestBytes = len(body)
	return result, nil
}

// request builds the HTTP request: the key as a bearer token only when one
// is configured.
func (c *Client) request(ctx context.Context, body []byte, stream bool) (*http.Request, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("could not create " + c.provider + " request")
	}
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	}
	return httpReq, nil
}

// httpError classifies an HTTP error reply by its status and code and keeps
// its message, without the client's key.
func (c *Client) httpError(response *http.Response) *provider.ProviderError {
	message, code := provider.ReadErrorResponse(response.Body)
	err := classify(response.StatusCode, code)
	err.Message, err.Provider = message, c.provider
	return err.WithoutKey(c.apiKey)
}
