package application

import (
	"context"
	"errors"
	"reflect"

	"github.com/underpass-ai/AXLR/domain"
)

type CompleteModelUseCase struct{ Models ModelPort }

func (u CompleteModelUseCase) Execute(ctx context.Context, req domain.CompletionRequest) (domain.CompletionResult, error) {
	if err := req.Validate(); err != nil {
		return domain.CompletionResult{}, err
	}
	if modelPortIsNil(u.Models) {
		return domain.CompletionResult{}, errors.New("model port is nil")
	}
	return u.Models.Complete(ctx, req)
}

func modelPortIsNil(port any) bool {
	if port == nil {
		return true
	}
	value := reflect.ValueOf(port)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
