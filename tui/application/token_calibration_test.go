package application

import (
	"context"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
)

type calibrationRecorder struct {
	models               []root.ModelID
	requestBytes, tokens []int
}

func (c *calibrationRecorder) Observe(_ context.Context, model root.ModelID, requestBytes, promptTokens int) error {
	c.models = append(c.models, model)
	c.requestBytes = append(c.requestBytes, requestBytes)
	c.tokens = append(c.tokens, promptTokens)
	return nil
}

// Each answered request reports the body the adapter sent and the prompt
// tokens the provider counted, so the console learns the model's ratio.
func TestContinueTurnMeasuresTheModelsBytesPerToken(t *testing.T) {
	for _, tc := range []struct {
		name     string
		result   root.CompletionResult
		observed bool
	}{
		{"usage and size", root.CompletionResult{RequestBytes: 42000, Usage: &root.TokenUsage{PromptTokens: 10000}}, true},
		{"no usage", root.CompletionResult{RequestBytes: 42000}, false},
		{"no size", root.CompletionResult{Usage: &root.TokenUsage{PromptTokens: 10000}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := turnSession(t)
			store := &memoryStore{}
			recorder := &calibrationRecorder{}
			model := streamFunc(func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
				result := tc.result
				result.Message = root.Message{Role: root.RoleAssistant, Content: "done"}
				return result, nil
			})
			u := StartTurnUseCase{Catalog: &catalogStub{}, Store: store, Continue: ContinueTurnUseCase{Models: model, Store: store, Calibration: recorder}}
			if err := u.Execute(context.Background(), &s, "hello", ignoreEvent); err != nil {
				t.Fatal(err)
			}
			if !tc.observed {
				if len(recorder.models) != 0 {
					t.Fatalf("observed %+v", recorder)
				}
				return
			}
			if len(recorder.models) != 1 || recorder.models[0] != "test/model" || recorder.requestBytes[0] != 42000 || recorder.tokens[0] != 10000 {
				t.Fatalf("observed %+v", recorder)
			}
		})
	}
}
