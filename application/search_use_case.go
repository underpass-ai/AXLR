package application

import (
	"context"

	"github.com/underpass-ai/AXLR/domain"
)

// SearchPort walks the workspace without following symlinks.
type SearchPort interface {
	Search(context.Context, domain.SearchCommand) (domain.SearchResult, error)
	List(context.Context, domain.ListCommand) (domain.ListResult, error)
}

type SearchUseCase struct{ Files SearchPort }

func (u SearchUseCase) Execute(ctx context.Context, c domain.SearchCommand) (domain.SearchResult, error) {
	return u.Files.Search(ctx, c)
}

type ListUseCase struct{ Files SearchPort }

func (u ListUseCase) Execute(ctx context.Context, c domain.ListCommand) (domain.ListResult, error) {
	return u.Files.List(ctx, c)
}
