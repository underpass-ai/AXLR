package openrouter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

const modelCatalogURL = "https://openrouter.ai/api/v1/models?supported_parameters=tools&output_modalities=text"
const modelCatalogLimit = 8 << 20

var errMalformedCatalog = errors.New("malformed OpenRouter model catalog")

type ModelCatalog struct {
	APIKey     string
	HTTPClient *http.Client
}

func (c ModelCatalog) List(ctx context.Context) ([]domain.AvailableModel, error) {
	if strings.TrimSpace(c.APIKey) == "" {
		return nil, errors.New("OpenRouter API key is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, modelCatalogURL, nil)
	if err != nil {
		return nil, errors.New("could not create OpenRouter model catalog request")
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
			return nil, ctx.Err()
		}
		return nil, errors.New("OpenRouter model catalog transport failure")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenRouter model catalog request failed (HTTP %d)", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, modelCatalogLimit+1))
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("could not read OpenRouter model catalog")
	}
	if len(body) > modelCatalogLimit {
		return nil, errors.New("OpenRouter model catalog exceeds 8 MiB")
	}
	var catalog modelCatalogResponse
	if json.Unmarshal(body, &catalog) != nil {
		return nil, errMalformedCatalog
	}
	return catalog.models()
}
