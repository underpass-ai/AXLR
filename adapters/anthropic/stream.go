package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/underpass-ai/AXLR/adapters/provider"
	"github.com/underpass-ai/AXLR/domain"
)

// Stream emits validated text deltas and returns only a completed assistant
// message. A callback error terminates the stream immediately and is
// returned unchanged. A request that fails once sent returns its
// RequestBytes beside the error; never a message.
func (c *Client) Stream(ctx context.Context, req domain.CompletionRequest, onText func(domain.Text) error) (domain.CompletionResult, error) {
	if err := ctx.Err(); err != nil {
		return domain.CompletionResult{}, err
	}
	if onText == nil {
		return domain.CompletionResult{}, errors.New(c.provider + " stream requires a text callback")
	}
	wire, err := mapRequest(req, c.maxTokens, c.thinking)
	if err != nil {
		return domain.CompletionResult{}, err
	}
	wire.Stream = true
	body, err := json.Marshal(wire)
	if err != nil {
		return domain.CompletionResult{}, errors.New("could not encode " + c.provider + " request")
	}
	sent := domain.CompletionResult{RequestBytes: len(body), Provider: c.host}
	streamCtx, cancelStream := context.WithCancelCause(ctx)
	defer cancelStream(nil)
	idle := c.streamInactivityTimeout
	inactivityTimer := time.AfterFunc(idle, func() { cancelStream(provider.NewStreamTimeoutError(c.provider, false)) })
	defer inactivityTimer.Stop()
	maximumDurationTimer := time.AfterFunc(c.streamMaxDuration, func() { cancelStream(provider.NewStreamTimeoutError(c.provider, true)) })
	defer maximumDurationTimer.Stop()
	streamError := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var timeout *provider.StreamTimeoutError
		if errors.As(context.Cause(streamCtx), &timeout) {
			return timeout
		}
		return nil
	}
	httpReq, err := c.request(streamCtx, body, true)
	if err != nil {
		return domain.CompletionResult{}, err
	}
	response, err := c.http.Do(httpReq)
	if err != nil {
		if cause := streamError(); cause != nil {
			return sent, cause
		}
		return sent, &provider.TransportError{Cause: err, Provider: c.provider}
	}
	var closeOnce sync.Once
	closeBody := func() { closeOnce.Do(func() { _ = response.Body.Close() }) }
	stop := context.AfterFunc(streamCtx, closeBody)
	defer func() { stop(); closeBody() }()
	if cause := streamError(); cause != nil {
		return sent, cause
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return sent, c.httpError(response)
	}
	decoder := provider.NewSSEDecoder(response.Body, c.provider)
	accumulator := streamAccumulator{provider: c.provider}
	streamed := 0
	for {
		if cause := streamError(); cause != nil {
			return sent, cause
		}
		data, err := decoder.Next()
		if cause := streamError(); cause != nil {
			return sent, cause
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return sent, errors.New(c.provider + " stream ended without message_stop")
			}
			return sent, &provider.TransportError{Cause: err, Provider: c.provider}
		}
		inactivityTimer.Stop()
		deltas, err := accumulator.Add(data)
		if err != nil {
			var failed *provider.ProviderError
			if errors.As(err, &failed) {
				failed.Provider = c.provider
				failed.WithoutKey(c.apiKey)
			}
			return sent, err
		}
		for _, text := range deltas {
			if err := ctx.Err(); err != nil {
				return sent, err
			}
			if err := onText(text); err != nil {
				return sent, err
			}
			streamed += len(text)
		}
		if accumulator.Stopped() {
			result, err := accumulator.Result(c.host)
			if err != nil {
				return sent, err
			}
			// What the mapper added after the streamed text, the note on an
			// answer cut by the output limit or declined, is streamed like
			// the text.
			if len(result.Message.Content) > streamed {
				if err := ctx.Err(); err != nil {
					return sent, err
				}
				if err := onText(result.Message.Content[streamed:]); err != nil {
					return sent, err
				}
			}
			result.RequestBytes = len(body)
			return result, nil
		}
		inactivityTimer.Reset(idle)
	}
}
