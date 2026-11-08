package application

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// boundSession is a bot's session: one under an execution's policy, at
// its first turn when begun, else before any prompt.
func boundSession(t *testing.T, begun bool) domain.Session {
	t.Helper()
	s := turnSession(t)
	if begun {
		s = rememberSession(t, domain.ModeNormal)
	}
	policy, err := domain.NewExecutionPolicy(domain.PolicyObserve, []domain.PolicyRule{{Name: "local_read"}, {Name: "axlr_remember"}, {Name: "axlr_recall"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetPolicy(policy); err != nil {
		t.Fatal(err)
	}
	return s
}

// Under an execution's policy axlr_remember and axlr_recall take the
// session's own abouts only: on 8 November 2026 a bot read and wrote
// project:<other bot> and user:gx10a, and Haiku cited in one bot's task
// what another had written under the person's about.
func TestBoundSessionKeepsRememberAndRecallToItsOwnAbouts(t *testing.T) {
	s := boundSession(t, true)
	state := s.Export()
	project := "project:" + filepath.Base(string(state.Workspace))
	own := "ws:" + string(state.ID)
	recorder := &rememberRecorder{answer: map[string]any{"accepted": true}}
	recall := &recallStub{texts: map[string]string{project: "- the parser keeps UTC"}}
	u := HostToolUseCase{Memory: recorder, Recall: recall, Labels: &contextLabels{labels: map[domain.SessionID]domain.SessionLabel{}}}
	remember := func(about string) domain.ToolOutcome {
		outcome, err := u.Execute(context.Background(), s, rememberIdentity(t), mustObject(t, `{"kind":"observation","text":"x","evidence":"e","about":"`+about+`"}`))
		if err != nil {
			t.Fatal(err)
		}
		return outcome
	}
	read := func(about string) domain.ToolOutcome {
		outcome, err := u.Execute(context.Background(), s, recallIdentity(t), mustObject(t, `{"about":"`+about+`","mode":"wake"}`))
		if err != nil {
			t.Fatal(err)
		}
		return outcome
	}
	for _, about := range []string{project, own} {
		if outcome := remember(about); outcome.IsError {
			t.Fatalf("own about %s refused: %s", about, outcome.Content)
		}
		if outcome := read(about); outcome.IsError {
			t.Fatalf("own about %s refused: %s", about, outcome.Content)
		}
	}
	for _, about := range []string{"project:other-bot", "user:gx10a", "ws:ffffffffffffffffffffffffffffffff"} {
		if outcome := remember(about); !outcome.IsError || !strings.Contains(string(outcome.Content), "keeps memory to its own abouts ("+project+", "+own+")") || !strings.Contains(string(outcome.Content), about+" is another's") {
			t.Fatalf("remember %s: %+v", about, outcome)
		}
		if outcome := read(about); !outcome.IsError || !strings.Contains(string(outcome.Content), about+" is another's") {
			t.Fatalf("recall %s: %+v", about, outcome)
		}
	}
	if len(recorder.requests) != 2 || len(recall.requests) != 2 {
		t.Fatalf("KMP was called for another's about: %d writes, %d reads", len(recorder.requests), len(recall.requests))
	}
	// A session that is nobody's bot names what it likes, as before.
	free := rememberSession(t, domain.ModeNormal)
	if outcome, _ := u.Execute(context.Background(), free, rememberIdentity(t), mustObject(t, `{"kind":"observation","text":"x","evidence":"e","about":"project:other-bot"}`)); outcome.IsError {
		t.Fatalf("free session refused: %s", outcome.Content)
	}
}

// listingStore lists the workspace's sessions, as the console's does.
type listingStore struct {
	memoryStore
	summaries []domain.SessionSummary
}

func (s *listingStore) List(context.Context) ([]domain.SessionSummary, error) {
	return s.summaries, nil
}

// A bot's first prompt wakes its project and its previous task's ws:, and
// never the person's about: each execution is a new session, so the ws:
// abouts of 1 093 bot sessions had never been read (8 November 2026).
func TestBoundSessionWakesItsPreviousTaskAndNotThePerson(t *testing.T) {
	s := boundSession(t, false)
	state := s.Export()
	project := "project:" + filepath.Base(string(state.Workspace))
	at := func(hour int) time.Time { return time.Date(2026, 11, 8, hour, 0, 0, 0, time.UTC) }
	store := &listingStore{summaries: []domain.SessionSummary{
		{ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Workspace: state.Workspace, MessageCount: 4, UpdatedAt: at(9)},
		{ID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Workspace: state.Workspace, MessageCount: 2, UpdatedAt: at(11)},
		{ID: "cccccccccccccccccccccccccccccccc", Workspace: state.Workspace, MessageCount: 0, UpdatedAt: at(12)},
		{ID: "dddddddddddddddddddddddddddddddd", Workspace: "/elsewhere", MessageCount: 3, UpdatedAt: at(13)},
		{ID: state.ID, Workspace: state.Workspace, MessageCount: 1, UpdatedAt: at(14)},
	}}
	recall := &recallStub{texts: map[string]string{"user:tirso": "- prefers commits in English", "ws:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb": "- task 2 found the leak in api-3"}}
	c := recallCeremony(t, recall)
	c.Store = store
	note := c.Recall(context.Background(), &s, "check the api pods again")
	var abouts []string
	for _, request := range recall.requests {
		abouts = append(abouts, request.About)
	}
	if got := strings.Join(abouts, " "); got != project+" ws:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("woke %q", got)
	}
	if strings.Contains(note, "user:tirso") || !strings.Contains(note, "Memory of ws:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb:\n- task 2 found the leak in api-3") {
		t.Fatalf("note %q", note)
	}
	// The first task of a bot has nothing earlier to wake.
	first := boundSession(t, false)
	recall.requests = nil
	c.Store = &listingStore{}
	c.Recall(context.Background(), &first, "check the api pods")
	if len(recall.requests) != 1 || recall.requests[0].About != "project:"+filepath.Base(string(first.Export().Workspace)) {
		t.Fatalf("first task woke %+v", recall.requests)
	}
	// A person's session keeps waking the project and the person.
	person := turnSession(t)
	recall.requests = nil
	c.Store = store
	c.Recall(context.Background(), &person, "check the api pods again")
	if len(recall.requests) != 2 || recall.requests[1].About != "user:tirso" {
		t.Fatalf("person woke %+v", recall.requests)
	}
}

// What Jev settles as a preference in a bot's turn stays in the bot's
// about: the person's memory is not the bot's to write.
func TestBoundSessionSettlesAPreferenceInItsOwnAbout(t *testing.T) {
	c, _, _, _ := sessionCeremonyFor(t, &fakeEngine{})
	c.Identity = &MemoryIdentity{Store: "/s", RegistrationKey: "k", Agents: &agentsStub{agents: map[string]string{}}, Guide: &guideStub{}, UserAbout: "user:tirso"}
	c.Judge = &scriptedJudge{gate: 0.9, sentence: "Las fechas del parser se guardan en UTC, como pide la RFC 3339", sentenceP: 0.8, kind: "preference", replaces: "none", replacesP: 0.9}
	s := turnSession(t)
	policy, err := domain.NewExecutionPolicy(domain.PolicyActWithinPolicy, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetPolicy(policy); err != nil {
		t.Fatal(err)
	}
	if err := s.BeginTurn("añade hola.txt y comprueba", turnTools()); err != nil {
		t.Fatal(err)
	}
	c.BeginTurn(context.Background(), &s)
	settleTurnOn(t, &s, settleAnswer)
	if err := c.EndTurn(context.Background(), &s); err != nil {
		t.Fatal(err)
	}
	journal := c.Journal.(*journalStub)
	project := "project:" + filepath.Base(string(s.Export().Workspace))
	if len(journal.settled) != 1 || journal.settled[0].About != project || journal.settled[0].Kind != "preference" {
		t.Fatalf("%+v", journal.settled)
	}
}
