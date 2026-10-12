package axlr

import (
	"context"
	"errors"
	"strings"
	"testing"

	rootApplication "github.com/underpass-ai/AXLR/application"
	root "github.com/underpass-ai/AXLR/domain"
)

type failingModels struct{ err error }

func (f failingModels) Stream(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
	return root.CompletionResult{}, f.err
}

// A failed request reaches the app log with the provider's message; a
// cancelled one does not, since the person cancelled it.
func TestModelStreamLogsFailedRequests(t *testing.T) {
	req := root.CompletionRequest{Model: "anthropic/claude-nonexistent-9", Messages: []root.Message{{Role: root.RoleUser, Content: "hola"}}}
	var logged []string
	for _, err := range []error{errors.New("OpenRouter invalid_request (HTTP 400): anthropic/claude-nonexistent-9 is not a valid model ID"), context.Canceled} {
		stream := ModelStream{UseCase: rootApplication.StreamModelUseCase{Models: failingModels{err: err}}, LogFailure: func(line string) { logged = append(logged, line) }}
		if _, got := stream.Stream(context.Background(), req, func(root.Text) error { return nil }); !errors.Is(got, err) {
			t.Fatalf("error %v", got)
		}
	}
	if len(logged) != 1 || !strings.Contains(logged[0], "anthropic/claude-nonexistent-9 failed: OpenRouter invalid_request (HTTP 400): anthropic/claude-nonexistent-9 is not a valid model ID") {
		t.Fatalf("logged %q", logged)
	}
}
