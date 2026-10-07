// Package typesafe is the TypeSafe Jev judgement adapter: one typed question
// about a state, answered with probabilities. It mirrors KMP's client: a
// pinned model, a bounded body, no redirects and at most two retries on 429.
package typesafe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/underpass-ai/AXLR/tui/application"
)

// Endpoint is TypeSafe's judgement API.
const Endpoint = "https://api.typesafe.ai/v1/systemone"

// DefaultModel is the version KMP pins; never a -latest alias.
const DefaultModel = "jev-1.13.0"

const (
	maxBodyBytes  = 256 << 10
	maxRetries    = 2
	maxRetryWait  = 5 * time.Second
	questionKey   = "q"
	defaultTimout = 20 * time.Second
)

var pinnedModel = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

type Config struct {
	APIKey  string
	Model   string
	Timeout time.Duration
	// Endpoint and HTTPClient exist for tests; empty means TypeSafe's API
	// and a client without redirects.
	Endpoint   string
	HTTPClient *http.Client
}

type Client struct {
	key, model, endpoint string
	http                 http.Client
}

var _ application.JudgementPort = (*Client)(nil)

func New(config Config) (*Client, error) {
	key := strings.TrimSpace(config.APIKey)
	if key == "" || strings.ContainsAny(key, "\r\n") {
		return nil, errors.New("TYPESAFE_API_KEY is required for Jev")
	}
	model := config.Model
	if model == "" {
		model = DefaultModel
	}
	if !pinnedModel.MatchString(model) || strings.HasSuffix(model, "-latest") {
		return nil, errors.New("jev.model must be a pinned version such as jev-1.13.0")
	}
	timeout := config.Timeout
	if timeout == 0 {
		timeout = defaultTimout
	}
	if timeout < time.Second || timeout > time.Minute {
		return nil, errors.New("jev.timeout_ms must be between 1000 and 60000")
	}
	endpoint := config.Endpoint
	if endpoint == "" {
		endpoint = Endpoint
	}
	client := http.Client{}
	if config.HTTPClient != nil {
		client = *config.HTTPClient
	}
	client.Timeout = timeout
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{key: key, model: model, endpoint: endpoint, http: client}, nil
}

func (c *Client) Judge(ctx context.Context, question application.JudgementQuestion) (application.JudgementVerdict, error) {
	if err := question.Validate(); err != nil {
		return application.JudgementVerdict{}, err
	}
	typed := map[string]any{"type": "noul", "instructions": question.Question}
	if len(question.Options) > 0 {
		criteria := make(map[string]any, len(question.Options))
		for _, option := range question.Options {
			criteria[option] = nil
		}
		typed = map[string]any{"type": "choice", "instructions": question.Question, "criteria": criteria}
	}
	body, err := json.Marshal(map[string]any{"state": question.State, "model": c.model, "questions": map[string]any{questionKey: typed}})
	if err != nil || len(body) > maxBodyBytes {
		return application.JudgementVerdict{}, errors.New("Jev request is too large")
	}
	data, err := c.post(ctx, body)
	if err != nil {
		return application.JudgementVerdict{}, err
	}
	return c.verdict(data, question)
}

func (c *Client) post(ctx context.Context, body []byte) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, errors.New("could not create the Jev request")
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.http.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, errors.New("TypeSafe transport failure")
		}
		data, readErr := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
		_ = resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusTooManyRequests && attempt < maxRetries:
			wait := time.Second
			if seconds, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && seconds >= 0 {
				wait = min(time.Duration(seconds)*time.Second, maxRetryWait)
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
			continue
		case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
			return nil, fmt.Errorf("TypeSafe rejected the API key (HTTP %d)", resp.StatusCode)
		case resp.StatusCode == http.StatusTooManyRequests:
			return nil, errors.New("TypeSafe rate limit persisted after retries")
		case resp.StatusCode < 200 || resp.StatusCode >= 300:
			return nil, fmt.Errorf("TypeSafe HTTP %d", resp.StatusCode)
		case readErr != nil:
			return nil, errors.New("TypeSafe transport failure")
		case len(data) > maxBodyBytes:
			return nil, errors.New("TypeSafe response exceeds 256 KiB")
		}
		return data, nil
	}
}

func (c *Client) verdict(data []byte, question application.JudgementQuestion) (application.JudgementVerdict, error) {
	var wire struct {
		Model   string                     `json:"model"`
		Answers map[string]json.RawMessage `json:"answers"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return application.JudgementVerdict{}, errors.New("invalid TypeSafe response")
	}
	if wire.Model != c.model {
		return application.JudgementVerdict{}, errors.New("TypeSafe answered with a different model than configured")
	}
	raw, ok := wire.Answers[questionKey]
	if !ok || len(wire.Answers) != 1 {
		return application.JudgementVerdict{}, errors.New("TypeSafe answers do not match the question sent")
	}
	var answer struct {
		Type          string             `json:"type"`
		Noul          *float64           `json:"noul"`
		Choice        string             `json:"choice"`
		Probabilities map[string]float64 `json:"probabilities"`
		Confidence    *float64           `json:"confidence"`
	}
	if err := json.Unmarshal(raw, &answer); err != nil {
		return application.JudgementVerdict{}, errors.New("invalid TypeSafe answer")
	}
	verdict := application.JudgementVerdict{Model: wire.Model}
	if len(question.Options) == 0 {
		if answer.Type != "noul" || !probability(answer.Noul) {
			return application.JudgementVerdict{}, errors.New("TypeSafe returned an invalid yes/no answer")
		}
		verdict.Yes = answer.Noul
		return verdict, nil
	}
	if answer.Type != "choice" || !probability(answer.Confidence) || len(answer.Probabilities) != len(question.Options) {
		return application.JudgementVerdict{}, errors.New("TypeSafe returned an invalid choice answer")
	}
	offered := false
	for _, option := range question.Options {
		p, ok := answer.Probabilities[option]
		if !ok || !probability(&p) {
			return application.JudgementVerdict{}, errors.New("TypeSafe probabilities do not cover the options offered")
		}
		offered = offered || option == answer.Choice
	}
	if !offered {
		return application.JudgementVerdict{}, errors.New("TypeSafe chose an option that was not offered")
	}
	verdict.Choice, verdict.Probabilities, verdict.Confidence = answer.Choice, answer.Probabilities, answer.Confidence
	return verdict, nil
}

func probability(p *float64) bool {
	return p != nil && !math.IsNaN(*p) && *p >= 0 && *p <= 1
}
