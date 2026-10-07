// Package localmodels routes model requests to OpenAI-compatible servers
// configured in settings.json and merges them into the /model catalog. The
// session keeps AXLR's model id; only the wire request names the server's.
package localmodels

import (
	"context"
	"fmt"

	root "github.com/underpass-ai/AXLR/domain"
)

// Client is a chat endpoint, such as the root OpenRouter client pointed at a
// local server.
type Client interface {
	Complete(context.Context, root.CompletionRequest) (root.CompletionResult, error)
	Stream(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error)
}

// Route sends a configured model to its server under the server's name.
type Route struct {
	Client   Client
	Upstream root.ModelID
}

// Router dispatches by model id: configured local models to their server,
// every other id to Default (OpenRouter), which is nil without a key.
type Router struct {
	Default Client
	Routes  map[root.ModelID]Route
}

func (r Router) Complete(ctx context.Context, req root.CompletionRequest) (root.CompletionResult, error) {
	client, req, err := r.route(req)
	if err != nil {
		return root.CompletionResult{}, err
	}
	return client.Complete(ctx, req)
}

func (r Router) Stream(ctx context.Context, req root.CompletionRequest, onText func(root.Text) error) (root.CompletionResult, error) {
	client, req, err := r.route(req)
	if err != nil {
		return root.CompletionResult{}, err
	}
	return client.Stream(ctx, req, onText)
}

func (r Router) route(req root.CompletionRequest) (Client, root.CompletionRequest, error) {
	if route, ok := r.Routes[req.Model]; ok && route.Client != nil {
		if route.Upstream != "" {
			req.Model = route.Upstream
		}
		return route.Client, req, nil
	}
	if r.Default == nil {
		return nil, req, fmt.Errorf("model %s is not a configured local model and OPENROUTER_API_KEY is not set", req.Model)
	}
	return r.Default, req, nil
}
