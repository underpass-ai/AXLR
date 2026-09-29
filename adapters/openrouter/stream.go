package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"

	"github.com/underpass-ai/AXLR/domain"
)

// Stream emits validated text deltas and returns only a completed assistant message.
// A callback error terminates the stream immediately and is returned unchanged.
func (c *Client) Stream(ctx context.Context, req domain.CompletionRequest, onText func(domain.Text) error) (domain.CompletionResult, error) {
	if err := ctx.Err(); err != nil {
		return domain.CompletionResult{}, err
	}
	if onText == nil {
		return domain.CompletionResult{}, errors.New("OpenRouter stream requires a text callback")
	}
	wire, err := mapRequest(req)
	if err != nil {
		return domain.CompletionResult{}, err
	}
	wire.Stream = true
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
	httpReq.Header.Set("Accept", "text/event-stream")
	response, err := c.http.Do(httpReq)
	if err != nil {
		if ctx.Err() != nil {
			return domain.CompletionResult{}, ctx.Err()
		}
		return domain.CompletionResult{}, &TransportError{Cause: err}
	}
	var closeOnce sync.Once
	closeBody := func() { closeOnce.Do(func() { _ = response.Body.Close() }) }
	stop := context.AfterFunc(ctx, closeBody)
	defer func() { stop(); closeBody() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return domain.CompletionResult{}, classifyProviderError(response.StatusCode)
	}
	decoder := newSSEDecoder(response.Body)
	var accumulator streamAccumulator
	for {
		if err := ctx.Err(); err != nil {
			return domain.CompletionResult{}, err
		}
		data, err := decoder.Next()
		if ctx.Err() != nil {
			return domain.CompletionResult{}, ctx.Err()
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return domain.CompletionResult{}, errors.New("OpenRouter stream ended without [DONE]")
			}
			return domain.CompletionResult{}, &TransportError{Cause: err}
		}
		if bytes.Equal(data, []byte("[DONE]")) {
			return accumulator.Result()
		}
		deltas, err := accumulator.Add(data)
		if err != nil {
			return domain.CompletionResult{}, err
		}
		for _, text := range deltas {
			if err := ctx.Err(); err != nil {
				return domain.CompletionResult{}, err
			}
			if err := onText(text); err != nil {
				return domain.CompletionResult{}, err
			}
		}
	}
}
