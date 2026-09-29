package application

import (
	"context"
	"errors"
	"testing"

	"github.com/underpass-ai/AXLR/domain"
)

type modelPortFunc func(context.Context, domain.CompletionRequest) (domain.CompletionResult, error)

func (f modelPortFunc) Complete(ctx context.Context, req domain.CompletionRequest) (domain.CompletionResult, error) {
	return f(ctx, req)
}

type nilPointerModelPort struct{ calls int }

func (p *nilPointerModelPort) Complete(context.Context, domain.CompletionRequest) (domain.CompletionResult, error) {
	p.calls++
	return domain.CompletionResult{}, nil
}

func TestCompleteModelUseCaseRejectsBeforeCallingPort(t *testing.T) {
	calls := 0
	u := CompleteModelUseCase{Models: modelPortFunc(func(context.Context, domain.CompletionRequest) (domain.CompletionResult, error) {
		calls++
		return domain.CompletionResult{}, nil
	})}
	if _, err := u.Execute(context.Background(), domain.CompletionRequest{}); err == nil {
		t.Fatal("invalid request accepted")
	}
	if calls != 0 {
		t.Fatalf("port called %d times for invalid request", calls)
	}
	valid := domain.CompletionRequest{
		Model:    "openai/gpt-4o",
		Messages: []domain.Message{{Role: domain.RoleUser, Content: "Hi"}},
	}
	if _, err := (CompleteModelUseCase{}).Execute(context.Background(), valid); err == nil {
		t.Fatal("nil port accepted")
	}
}

func TestCompleteModelUseCaseForwardsAndPropagates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := domain.CompletionRequest{
		Model:    "openai/gpt-4o",
		Messages: []domain.Message{{Role: domain.RoleUser, Content: "Hi"}},
	}
	want := domain.CompletionResult{
		Message:      domain.Message{Role: domain.RoleAssistant, Content: "Hello"},
		FinishReason: "stop",
	}
	calls := 0
	u := CompleteModelUseCase{Models: modelPortFunc(func(gotCtx context.Context, gotReq domain.CompletionRequest) (domain.CompletionResult, error) {
		calls++
		if gotCtx != ctx || gotReq.Model != req.Model || gotReq.Messages[0].Content != "Hi" {
			t.Fatalf("port received wrong context or request: %+v", gotReq)
		}
		return want, nil
	})}
	got, err := u.Execute(ctx, req)
	if err != nil || got.Message.Content != "Hello" || got.FinishReason != "stop" || calls != 1 {
		t.Fatalf("completion = %+v, %v; calls = %d", got, err, calls)
	}

	portFailure := errors.New("provider failed")
	u.Models = modelPortFunc(func(context.Context, domain.CompletionRequest) (domain.CompletionResult, error) {
		return domain.CompletionResult{}, portFailure
	})
	if _, err := u.Execute(ctx, req); !errors.Is(err, portFailure) {
		t.Fatalf("port error changed: %v", err)
	}
}

func TestCompleteModelUseCaseRejectsTypedNilPort(t *testing.T) {
	var nilPort *nilPointerModelPort
	u := CompleteModelUseCase{Models: nilPort}
	valid := domain.CompletionRequest{
		Model:    "openai/gpt-4o",
		Messages: []domain.Message{{Role: domain.RoleUser, Content: "Hi"}},
	}
	if _, err := u.Execute(context.Background(), valid); err == nil {
		t.Fatal("typed nil model port accepted")
	}
}
