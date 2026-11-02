package openrouter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

const generationURL = "https://openrouter.ai/api/v1/generation?id="
const generationLimit = 64 << 10

// generationTimeout bounds one lookup; the use case bounds them all.
const generationTimeout = 2 * time.Second

var errMalformedGeneration = errors.New("malformed OpenRouter generation stats")

var _ application.GenerationCostPort = GenerationCosts{}

// GenerationCosts reads a request's cost from OpenRouter's generation
// endpoint (GET /api/v1/generation?id=…), with the key the model client
// uses. A 404 is stats not ready yet, which OpenRouter answers for a while
// after a request ends.
type GenerationCosts struct {
	APIKey     string
	HTTPClient *http.Client
}

func (c GenerationCosts) GenerationCost(ctx context.Context, id string) (domain.GenerationCost, error) {
	if strings.TrimSpace(c.APIKey) == "" {
		return domain.GenerationCost{}, errors.New("OpenRouter API key is required")
	}
	if strings.TrimSpace(id) == "" {
		return domain.GenerationCost{}, errors.New("OpenRouter generation id is required")
	}
	if err := ctx.Err(); err != nil {
		return domain.GenerationCost{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, generationTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, generationURL+url.QueryEscape(id), nil)
	if err != nil {
		return domain.GenerationCost{}, errors.New("could not create OpenRouter generation request")
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	client := http.DefaultClient
	if c.HTTPClient != nil {
		client = c.HTTPClient
	}
	copyClient := *client
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := copyClient.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return domain.GenerationCost{}, ctx.Err()
		}
		return domain.GenerationCost{}, errors.New("OpenRouter generation transport failure")
	}
	defer response.Body.Close()
	switch {
	case response.StatusCode == http.StatusNotFound:
		return domain.GenerationCost{}, application.ErrGenerationPending
	case response.StatusCode != http.StatusOK:
		return domain.GenerationCost{}, fmt.Errorf("OpenRouter generation request failed (HTTP %d)", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, generationLimit+1))
	if err != nil {
		if ctx.Err() != nil {
			return domain.GenerationCost{}, ctx.Err()
		}
		return domain.GenerationCost{}, errors.New("could not read OpenRouter generation stats")
	}
	if len(body) > generationLimit {
		return domain.GenerationCost{}, errors.New("OpenRouter generation stats exceed 64 KiB")
	}
	var stats generationResponse
	if json.Unmarshal(body, &stats) != nil {
		return domain.GenerationCost{}, errMalformedGeneration
	}
	return stats.cost()
}
