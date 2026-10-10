package application

import (
	"context"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// On 10 October 2026 claude-haiku-5.5 loaded the axlr-session skill and
// never set a title over seven prompts; Ctrl+O listed two sessions by the
// same first prompt. The console now titles an untitled session itself once
// its second request completes, from the person's first two prompts, without
// a model request and without changing the system prompt.
func TestConsoleTitlesAnUntitledSessionAfterTheSecondRequest(t *testing.T) {
	s := turnSession(t)
	labels := &contextLabels{labels: map[domain.SessionID]domain.SessionLabel{}}
	var prompts []root.Text
	u := AgentTurnUseCase{Continue: ContinueTurnUseCase{Store: &memoryStore{}, SessionLabels: labels, Models: streamFunc(func(_ context.Context, req root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		prompts = append(prompts, req.Messages[0].Content)
		return assistant("Hecho"), nil
	})}}
	id := s.Export().ID
	for i, prompt := range []string{"Revisa el error de permisos\nde mcp.json al arrancar", "[AXLR · memory] console note", "Añade el modo al mensaje", "Y los tests"} {
		if err := s.BeginTurn(root.Text(prompt), HostTools()); err != nil {
			t.Fatal(err)
		}
		if err := u.Execute(context.Background(), &s, nil); err != nil {
			t.Fatal(err)
		}
		if title := labels.labels[id].Title; i < 2 && title != "" {
			t.Fatalf("titled before the second request completed: %q", title)
		}
	}
	if title := labels.labels[id].Title; title != "Revisa el error de permisos · Añade el modo al mensaje" {
		t.Fatalf("console title %q", title)
	}
	if len(prompts) != 4 || prompts[0] != prompts[3] {
		t.Fatalf("a title changed the system prompt:\n%s\n---\n%s", prompts[0], prompts[len(prompts)-1])
	}
}

func TestConsoleTitleNeverReplacesOneAlreadySet(t *testing.T) {
	s := turnSession(t)
	id := s.Export().ID
	labels := &contextLabels{labels: map[domain.SessionID]domain.SessionLabel{id: {Title: "Título manual"}}}
	u := AgentTurnUseCase{Continue: ContinueTurnUseCase{Store: &memoryStore{}, SessionLabels: labels, Models: streamFunc(func(context.Context, root.CompletionRequest, func(root.Text) error) (root.CompletionResult, error) {
		return assistant("Hecho"), nil
	})}}
	for _, prompt := range []string{"uno", "dos"} {
		if err := s.BeginTurn(root.Text(prompt), HostTools()); err != nil {
			t.Fatal(err)
		}
		if err := u.Execute(context.Background(), &s, nil); err != nil {
			t.Fatal(err)
		}
	}
	if title := labels.labels[id].Title; title != "Título manual" {
		t.Fatalf("title replaced: %q", title)
	}
}

func TestDerivedTitleIsShortAndSingleLine(t *testing.T) {
	long := strings.Repeat("palabra ", 30)
	title := derivedTitle([]string{long, "  \n\t segunda   petición\ncon detalle"})
	if strings.ContainsAny(title, "\n\t") || len([]rune(title)) > domain.MaxSessionTitleRunes || !strings.HasSuffix(title, " · segunda petición") || !strings.Contains(title, "…") {
		t.Fatalf("derived title %q", title)
	}
	if derivedTitle([]string{"Arregla el parser", "Arregla el parser"}) != "Arregla el parser" {
		t.Fatal("a repeated prompt is not repeated in the title")
	}
}
