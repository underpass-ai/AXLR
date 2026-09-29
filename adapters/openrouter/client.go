package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/underpass-ai/AXLR/domain"
)

const endpoint = "https://openrouter.ai/api/v1/chat/completions"
const maxResponseBytes = 8 * 1024 * 1024

type Client struct {
	apiKey                  string
	http                    http.Client
	streamInactivityTimeout time.Duration
}

const defaultStreamInactivityTimeout = time.Minute

func New(config ClientConfig) (*Client, error) {
	if strings.TrimSpace(config.APIKey) == "" || strings.ContainsAny(config.APIKey, "\r\n") {
		return nil, errors.New("OpenRouter API key is required")
	}
	if config.StreamInactivityTimeout < 0 {
		return nil, errors.New("OpenRouter stream inactivity timeout must not be negative")
	}
	streamInactivityTimeout := config.StreamInactivityTimeout
	if streamInactivityTimeout == 0 {
		streamInactivityTimeout = defaultStreamInactivityTimeout
	}
	client := http.Client{}
	if config.HTTPClient != nil {
		client = *config.HTTPClient
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{apiKey: config.APIKey, http: client, streamInactivityTimeout: streamInactivityTimeout}, nil
}

func (c *Client) Complete(ctx context.Context, req domain.CompletionRequest) (domain.CompletionResult, error) {
	if err := ctx.Err(); err != nil {
		return domain.CompletionResult{}, err
	}
	wire, err := mapRequest(req)
	if err != nil {
		return domain.CompletionResult{}, err
	}
	body, err := json.Marshal(wire)
	if err != nil {
		return domain.CompletionResult{}, errors.New("could not encode OpenRouter request")
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return domain.CompletionResult{}, errors.New("could not create OpenRouter request")
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpResp, err := c.http.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return domain.CompletionResult{}, ctx.Err()
		}
		return domain.CompletionResult{}, &TransportError{Cause: err}
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return domain.CompletionResult{}, classifyProviderError(httpResp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(httpResp.Body, maxResponseBytes+1))
	if err != nil {
		return domain.CompletionResult{}, &TransportError{Cause: err}
	}
	if len(data) > maxResponseBytes {
		return domain.CompletionResult{}, errors.New("OpenRouter response exceeds 8 MiB")
	}
	var reply responseDTO
	if err := json.Unmarshal(data, &reply); err != nil {
		return domain.CompletionResult{}, errors.New("malformed OpenRouter response")
	}
	return mapResponse(reply)
}
