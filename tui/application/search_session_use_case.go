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
	archived := 0
	for i, m := range state.Messages {
		if strings.Contains(strings.ToLower(string(m.Content)), needle) {
			hits = append(hits, domain.SearchHit{MessageIndex: &i, Content: m.Content})
		}
		for archived < len(state.ArchivedDrafts) && state.ArchivedDrafts[archived].AfterMessage == i+1 {
			content := state.ArchivedDrafts[archived].Content
			if strings.Contains(strings.ToLower(string(content)), needle) {
				index := archived
				hits = append(hits, domain.SearchHit{ArchivedDraftIndex: &index, Content: content})
			}
			archived++
		}
	}
	if state.Draft != "" && strings.Contains(strings.ToLower(string(state.Draft)), needle) {
		hits = append(hits, domain.SearchHit{Draft: true, Content: state.Draft})
	}
	return hits
}
