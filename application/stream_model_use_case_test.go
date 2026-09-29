package application

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/underpass-ai/AXLR/domain"
)

type modelStreamPortFunc func(context.Context, domain.CompletionRequest, func(domain.Text) error) (domain.CompletionResult, error)

func (f modelStreamPortFunc) Stream(ctx context.Context, req domain.CompletionRequest, emit func(domain.Text) error) (domain.CompletionResult, error) {
	return f(ctx, req, emit)
}

type nilPointerModelStreamPort struct{}

func (*nilPointerModelStreamPort) Stream(context.Context, domain.CompletionRequest, func(domain.Text) error) (domain.CompletionResult, error) {
	panic("typed nil stream port called")
}

func validStreamRequest() domain.CompletionRequest {
	return domain.CompletionRequest{
		Model:    "openai/gpt-4o",
		Messages: []domain.Message{{Role: domain.RoleUser, Content: "Hi"}},
	}
}

func TestStreamModelUseCaseRejectsInvalidRequest(t *testing.T) {
	calls := 0
	u := StreamModelUseCase{Models: modelStreamPortFunc(func(context.Context, domain.CompletionRequest, func(domain.Text) error) (domain.CompletionResult, error) {
		calls++
		return domain.CompletionResult{}, nil
	})}
	if _, err := u.Execute(context.Background(), domain.CompletionRequest{}, func(domain.Text) error { return nil }); err == nil {
		t.Fatal("invalid request accepted")
	}
	if calls != 0 {
		t.Fatalf("port called %d times for invalid request", calls)
	}
}

func TestStreamModelUseCaseRejectsNilPort(t *testing.T) {
	req := validStreamRequest()
	if _, err := (StreamModelUseCase{}).Execute(context.Background(), req, func(domain.Text) error { return nil }); err == nil {
		t.Fatal("nil port accepted")
	}
	var typedNil *nilPointerModelStreamPort
	if _, err := (StreamModelUseCase{Models: typedNil}).Execute(context.Background(), req, func(domain.Text) error { return nil }); err == nil {
		t.Fatal("typed nil port accepted")
	}
}

func TestStreamModelUseCaseForwardsDeltasAndResult(t *testing.T) {
	ctx := context.WithValue(context.Background(), struct{}{}, "request")
	req := validStreamRequest()
	want := domain.CompletionResult{
		Message:      domain.Message{Role: domain.RoleAssistant, Content: "Hello"},
		FinishReason: "stop",
	}
	var gotDeltas []domain.Text
	calls := 0
	u := StreamModelUseCase{Models: modelStreamPortFunc(func(gotCtx context.Context, gotReq domain.CompletionRequest, emit func(domain.Text) error) (domain.CompletionResult, error) {
		calls++
		if gotCtx != ctx || !reflect.DeepEqual(gotReq, req) {
			t.Fatalf("port got context %v and request %+v", gotCtx, gotReq)
		}
		for _, delta := range []domain.Text{"Hel", "lo"} {
			if err := emit(delta); err != nil {
				return domain.CompletionResult{}, err
			}
		}
		return want, nil
	})}
	got, err := u.Execute(ctx, req, func(delta domain.Text) error {
		gotDeltas = append(gotDeltas, delta)
		return nil
	})
	if err != nil || !reflect.DeepEqual(got, want) || !reflect.DeepEqual(gotDeltas, []domain.Text{"Hel", "lo"}) || calls != 1 {
		t.Fatalf("result = %+v, err = %v, deltas = %v, calls = %d", got, err, gotDeltas, calls)
	}
}

func TestStreamModelUseCasePropagatesCallbackError(t *testing.T) {
	callbackErr := errors.New("display failed")
	emissions := 0
	u := StreamModelUseCase{Models: modelStreamPortFunc(func(_ context.Context, _ domain.CompletionRequest, emit func(domain.Text) error) (domain.CompletionResult, error) {
		for _, delta := range []domain.Text{"first", "second"} {
			emissions++
			if err := emit(delta); err != nil {
				return domain.CompletionResult{}, err
			}
		}
		return domain.CompletionResult{}, nil
	})}
	_, err := u.Execute(context.Background(), validStreamRequest(), func(domain.Text) error { return callbackErr })
	if !errors.Is(err, callbackErr) || emissions != 1 {
		t.Fatalf("error = %v, emissions = %d; want callback error after first delta", err, emissions)
	}
}
