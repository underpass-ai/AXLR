package application

import (
	"context"
	"errors"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type modelCatalogFunc func(context.Context) ([]domain.AvailableModel, error)

func (f modelCatalogFunc) List(ctx context.Context) ([]domain.AvailableModel, error) { return f(ctx) }

func catalogModel(id, name string, tools, text bool) domain.AvailableModel {
	return domain.AvailableModel{ID: root.ModelID(id), Name: root.Text(name), SupportsTools: tools, TextOutput: text}
}

func TestListModelsFiltersDeduplicatesAndSorts(t *testing.T) {
	models := []domain.AvailableModel{
		catalogModel("z/model", "Zulu", true, true),
		catalogModel("b/model", "Alpha", true, true),
		catalogModel("a/model", "Alpha", true, true),
		catalogModel("a/model", "Duplicate", true, true),
		catalogModel("no/tools", "Hidden", false, true),
		catalogModel("no/text", "Hidden", true, false),
		catalogModel("", "Invalid ID", true, true),
		catalogModel("bad\x00name", "Invalid ID", true, true),
		catalogModel("valid/bad-name", "bad\x00name", true, true),
	}
	u := ListModelsUseCase{Catalog: modelCatalogFunc(func(context.Context) ([]domain.AvailableModel, error) { return models, nil })}
	got, err := u.Execute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].ID != "a/model" || got[1].ID != "b/model" || got[2].ID != "z/model" || got[0].Name != "Alpha" {
		t.Fatalf("unexpected models: %+v", got)
	}
	got[0].Name = "changed"
	if models[2].Name != "Alpha" {
		t.Fatal("result aliases catalog slice")
	}
}

func TestListModelsRejectsNilCatalog(t *testing.T) {
	if _, err := (ListModelsUseCase{}).Execute(context.Background()); err == nil {
		t.Fatal("nil catalog accepted")
	}
}

func TestListModelsHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	u := ListModelsUseCase{Catalog: modelCatalogFunc(func(context.Context) ([]domain.AvailableModel, error) {
		t.Fatal("catalog called after cancellation")
		return nil, nil
	})}
	if _, err := u.Execute(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestListModelsReportsEmptyUsableCatalog(t *testing.T) {
	u := ListModelsUseCase{Catalog: modelCatalogFunc(func(context.Context) ([]domain.AvailableModel, error) {
		return []domain.AvailableModel{catalogModel("no/tools", "No tools", false, true)}, nil
	})}
	if _, err := u.Execute(context.Background()); err == nil {
		t.Fatal("empty usable catalog accepted")
	}
}

func TestListModelsPropagatesCatalogFailure(t *testing.T) {
	want := errors.New("catalog unavailable")
	u := ListModelsUseCase{Catalog: modelCatalogFunc(func(context.Context) ([]domain.AvailableModel, error) { return nil, want })}
	if _, err := u.Execute(context.Background()); !errors.Is(err, want) {
		t.Fatalf("got %v", err)
	}
}

func TestListModelsRejectsModelsWhenCanceledDuringCatalogList(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	u := ListModelsUseCase{Catalog: modelCatalogFunc(func(context.Context) ([]domain.AvailableModel, error) {
		cancel()
		return []domain.AvailableModel{catalogModel("provider/model", "Model", true, true)}, nil
	})}
	models, err := u.Execute(ctx)
	if !errors.Is(err, context.Canceled) || len(models) != 0 {
		t.Fatalf("canceled catalog returned models=%v error=%v", models, err)
	}
}
