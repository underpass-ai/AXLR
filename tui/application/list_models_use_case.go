package application

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

type ListModelsUseCase struct {
	Catalog     ModelCatalogPort
	Diagnostics DiagnosticPort
}

func (u ListModelsUseCase) Execute(ctx context.Context) (result []domain.AvailableModel, returnErr error) {
	if u.Catalog == nil {
		return nil, errors.New("model catalog is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	ctx, span := StartDiagnosticSpan(ctx, u.Diagnostics, DiagnosticActionCatalog, DiagnosticEvent{})
	defer func() {
		class := DiagnosticErrorNone
		if returnErr != nil {
			class = DiagnosticErrorProvider
			if errors.Is(returnErr, context.Canceled) {
				class = DiagnosticErrorCancelled
			} else if errors.Is(returnErr, context.DeadlineExceeded) {
				class = DiagnosticErrorTimeout
			}
		}
		span.End(class)
	}()
	started := time.Now()
	if u.Diagnostics != nil {
		_ = u.Diagnostics.Record(DiagnosticEvent{Stage: DiagnosticModelCatalogStart, SpanID: CurrentDiagnosticSpan(ctx)})
	}
	models, err := u.Catalog.List(ctx)
	if u.Diagnostics != nil {
		class := DiagnosticErrorNone
		if err != nil {
			class = DiagnosticErrorProvider
		}
		_ = u.Diagnostics.Record(DiagnosticEvent{Stage: DiagnosticModelCatalogDone, SpanID: CurrentDiagnosticSpan(ctx), ElapsedMilliseconds: time.Since(started).Milliseconds(), ErrorClass: class})
	}
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	usable := make([]domain.AvailableModel, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		if !model.SupportsTools || !model.TextOutput || model.Validate() != nil {
			continue
		}
		id := string(model.ID)
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		usable = append(usable, model)
	}
	if len(usable) == 0 {
		return nil, errors.New("no usable models available")
	}
	sort.Slice(usable, func(i, j int) bool {
		if usable[i].Name != usable[j].Name {
			return usable[i].Name < usable[j].Name
		}
		return usable[i].ID < usable[j].ID
	})
	return usable, nil
}
