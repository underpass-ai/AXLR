package application

import (
	"strings"

	axlr "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type SearchSessionUseCase struct{}

// Execute returns one hit per matching text block, in transcript order then draft.
func (SearchSessionUseCase) Execute(session domain.Session, query axlr.Text) []domain.SearchHit {
	if query == "" {
		return nil
	}
	needle := strings.ToLower(string(query))
	var hits []domain.SearchHit
	state := session.Export()
	for i, m := range state.Messages {
		if strings.Contains(strings.ToLower(string(m.Content)), needle) {
			hits = append(hits, domain.SearchHit{MessageIndex: &i, Content: m.Content})
		}
	}
	if state.Draft != "" && strings.Contains(strings.ToLower(string(state.Draft)), needle) {
		hits = append(hits, domain.SearchHit{Draft: true, Content: state.Draft})
	}
	return hits
}
