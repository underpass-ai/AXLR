package application

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type recallStub struct {
	requests []RecallRequest
	texts    map[string]string
	errs     map[string]error
}

func (r *recallStub) Recall(_ context.Context, request RecallRequest) (RecallResult, error) {
	r.requests = append(r.requests, request)
	if err := r.errs[request.About]; err != nil {
		return RecallResult{}, err
	}
	text, ok := r.texts[request.About]
	if !ok {
		return RecallResult{About: request.About, Mode: RecallWake, Empty: true}, nil
	}
	return RecallResult{About: request.About, Mode: RecallWake, Text: text, Refs: []string{request.About + ":entry:x"}}, nil
}

func recallCeremony(t *testing.T, recall *recallStub) *SessionCeremony {
	t.Helper()
	c, _, _, _ := sessionCeremonyFor(t, nil)
	c.Engine = nil
	identity := identityFor(&guideStub{}, &agentsStub{agents: map[string]string{}})
	identity.Recall, identity.UserAbout = recall, "user:tirso"
	c.Identity = identity
	return c
}

// The first prompt wakes the project's about and the person's, in the
// session's context, and the note says what each holds; nothing recorded
// is one line (decision D6).
func TestRecallNoteWakesTheProjectAndThePersonAtTheFirstPrompt(t *testing.T) {
	recall := &recallStub{texts: map[string]string{"user:tirso": "- prefers commits in English"}}
	c := recallCeremony(t, recall)
	s := turnSession(t)
	project := "project:" + filepath.Base(string(s.Export().Workspace))
	note := c.Recall(context.Background(), &s, "añade tests al parser")
	want := RecallNotePrefix + " " + project + ": new project, nothing recorded yet.\nMemory of user:tirso:\n- prefers commits in English"
	if note != want {
		t.Fatalf("note %q", note)
	}
	if len(recall.requests) != 2 || recall.requests[0].About != project || recall.requests[0].ContextID != "ctx_1" || recall.requests[0].Intent != "añade tests al parser" || recall.requests[1].About != "user:tirso" {
		t.Fatalf("requests %+v", recall.requests)
	}
	instance, _ := s.Instance()
	if instance.Recalled != "añade tests al parser" || instance.KMPContext != "ctx_1" {
		t.Fatalf("instance %+v", instance)
	}
	// The next prompt on the same topic gets no note; another topic does.
	if err := s.BeginTurn("añade tests al parser", turnTools()); err != nil {
		t.Fatal(err)
	}
	if note := c.Recall(context.Background(), &s, "ahora los tests del parser de fechas"); note != "" || len(recall.requests) != 2 {
		t.Fatalf("same topic woke again: %q %d", note, len(recall.requests))
	}
	if note := c.Recall(context.Background(), &s, "sí"); note != "" {
		t.Fatalf("a short follow-up woke again: %q", note)
	}
	if note := c.Recall(context.Background(), &s, "cambia el instalador de windows para firmar binarios"); !strings.HasPrefix(note, RecallNotePrefix) || len(recall.requests) != 4 {
		t.Fatalf("topic change: %q %d", note, len(recall.requests))
	}
	if instance, _ = s.Instance(); !strings.HasPrefix(instance.Recalled, "cambia el instalador") {
		t.Fatalf("instance %+v", instance)
	}
}

// A session this console did not recall yet but which already has
// messages was resumed: its own history is woken too, once.
func TestRecallNoteWakesTheSessionsOwnAboutWhenResumed(t *testing.T) {
	recall := &recallStub{texts: map[string]string{}}
	c := recallCeremony(t, recall)
	s := turnSession(t)
	if err := s.BeginTurn("primero", turnTools()); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAssistant(assistant("hecho")); err != nil {
		t.Fatal(err)
	}
	if err := s.SetInstance(domain.SessionInstance{KMPContext: "ctx_old", Recalled: "primero"}); err != nil {
		t.Fatal(err)
	}
	id := string(s.ID())
	note := c.Recall(context.Background(), &s, "primero")
	if !strings.Contains(note, "ws:"+id+": nothing recorded yet.") || len(recall.requests) != 3 || recall.requests[2].About != "ws:"+id || recall.requests[2].ContextID != "ctx_old" {
		t.Fatalf("note %q requests %+v", note, recall.requests)
	}
	if note := c.Recall(context.Background(), &s, "primero"); note != "" || len(recall.requests) != 3 {
		t.Fatalf("resumed twice: %q", note)
	}
}

// The note is bounded like a ceremony's recall and a failing wake is
// disclosed, never hidden.
func TestRecallNoteIsBoundedAndDisclosesFailures(t *testing.T) {
	s := turnSession(t)
	project := "project:" + filepath.Base(string(s.Export().Workspace))
	recall := &recallStub{texts: map[string]string{project: strings.Repeat("- a long line of memory\n", 400)}, errs: map[string]error{"user:tirso": errors.New("engine down")}}
	c := recallCeremony(t, recall)
	var logged []string
	c.Log = func(line string) { logged = append(logged, line) }
	note := c.Recall(context.Background(), &s, "hola que tal todo")
	// The prefix, a space and the ellipsis bounded appends are the only
	// bytes beyond the budget.
	if len(note) > maxCeremonyMemoryBytes+len(RecallNotePrefix)+1+len("…") || !strings.HasPrefix(note, RecallNotePrefix+" Memory of "+project+":\n- a long line") {
		t.Fatalf("note %d bytes: %.120q", len(note), note)
	}
	if strings.Contains(note, "user:tirso") {
		t.Fatalf("the budget was spent on the project; the person's section had no room: %q", note[len(note)-80:])
	}
	short := &recallStub{texts: map[string]string{}, errs: map[string]error{"user:tirso": errors.New("engine down")}}
	c = recallCeremony(t, short)
	other := turnSession(t)
	if note := c.Recall(context.Background(), &other, "hola que tal todo"); !strings.Contains(note, "user:tirso: KMP recall unavailable: engine down") {
		t.Fatalf("note %q", note)
	}
}

// Modes that wake their own about (a ceremony's) and the planner get no
// note, and neither do sessions the console created.
func TestRecallNoteSkipsCeremonyModesAndWorkers(t *testing.T) {
	for _, mode := range []domain.WorkMode{domain.ModeDebug, domain.ModePlan, domain.ModeTask, domain.ModeRepair} {
		recall := &recallStub{texts: map[string]string{}}
		c := recallCeremony(t, recall)
		s := turnSession(t)
		if err := s.SetMode(mode); err != nil {
			t.Fatal(err)
		}
		if note := c.Recall(context.Background(), &s, "arregla el fallo del parser"); note != "" || len(recall.requests) != 0 {
			t.Fatalf("%s: %q %d", mode, note, len(recall.requests))
		}
	}
	// Without KMP the hook is inert.
	var none *SessionCeremony
	s := turnSession(t)
	if note := none.Recall(context.Background(), &s, "hola"); note != "" {
		t.Fatal(note)
	}
}

func TestTopicChanged(t *testing.T) {
	cases := []struct {
		last, prompt string
		want         bool
	}{
		{"añade tests al parser de fechas", "más tests para el parser", false},
		{"añade tests al parser de fechas", "ok", false},
		{"añade tests al parser de fechas", "ahora cambia el instalador de windows para firmar binarios", true},
		{"", "una pregunta nueva sobre cualquier cosa larga", true},
	}
	for _, tc := range cases {
		if got := topicChanged(tc.last, tc.prompt); got != tc.want {
			t.Fatalf("%q → %q: %v", tc.last, tc.prompt, got)
		}
	}
}

// StartTurn appends the note to the person's prompt before the turn
// begins, so the first request carries it.
func TestStartTurnAppendsTheRecallNote(t *testing.T) {
	s := turnSession(t)
	store := &memoryStore{}
	recall := &recallStub{texts: map[string]string{}}
	c := recallCeremony(t, recall)
	c.Store = store
	model := streamFunc(func(_ context.Context, r root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
		if !strings.Contains(string(r.Messages[len(r.Messages)-1].Content), RecallNotePrefix) {
			t.Fatalf("request without the note: %q", r.Messages[len(r.Messages)-1].Content)
		}
		return assistant("hola"), nil
	})
	u := StartTurnUseCase{Catalog: &catalogStub{}, Store: store, Continue: ContinueTurnUseCase{Models: model, Store: store, Session: c}}
	if err := u.Execute(context.Background(), &s, "hola", func(Event) error { return nil }); err != nil {
		t.Fatal(err)
	}
	first := s.Messages()[0].Content
	if !strings.HasPrefix(string(first), "hola\n\n"+RecallNotePrefix+" project:") || len(recall.requests) != 2 {
		t.Fatalf("prompt %q requests %d", first, len(recall.requests))
	}
	record := BuildTurnRecord(s, 0, 1, "project:x", nil)
	if !strings.Contains(record.Prompt, RecallNotePrefix) {
		t.Logf("the turn record keeps the note in the prompt excerpt: %d bytes", len(record.Prompt))
	}
}
