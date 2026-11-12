package application

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
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
		if outcome := remember(about); !outcome.IsError || !strings.Contains(string(outcome.Content), `"error":"leave about out to write under `+project+", or name one of "+project+", "+own+":") || !strings.Contains(string(outcome.Content), about+" is another's") {
			t.Fatalf("remember %s: %+v", about, outcome)
		}
		if outcome := read(about); !outcome.IsError || !strings.Contains(string(outcome.Content), `"error":"name about as one of `+project+", "+own+":") || !strings.Contains(string(outcome.Content), about+" is another's") {
			t.Fatalf("recall %s: %+v", about, outcome)
		}
	}
	if len(recorder.requests) != 2 || len(recall.requests) != 2 {
		t.Fatalf("KMP was called for another's about: %d writes, %d reads", len(recorder.requests), len(recall.requests))
	}
	// An about selected with axlr_session (a read the policy allows) does
	// not move the boundary: a write without an about still lands in the
	// workspace's project about, and the selected one is another's.
	u.Labels = &contextLabels{labels: map[domain.SessionID]domain.SessionLabel{state.ID: {About: "project:other-bot"}}}
	if outcome, err := u.Execute(context.Background(), s, rememberIdentity(t), mustObject(t, `{"kind":"observation","text":"y","evidence":"e"}`)); err != nil || outcome.IsError || recorder.requests[len(recorder.requests)-1].About != project {
		t.Fatalf("selected about moved the boundary: %+v %v %+v", outcome, err, recorder.requests[len(recorder.requests)-1])
	}
	if outcome := remember("project:other-bot"); !outcome.IsError {
		t.Fatalf("selected about accepted: %+v", outcome)
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

// teamSession is a bot's session whose policy shares its memory with the
// team qa.
func teamSession(t *testing.T, begun bool) domain.Session {
	t.Helper()
	s := boundSession(t, begun)
	policy, _ := s.Policy()
	shared, err := policy.WithMemoryTeam("qa")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetPolicy(shared); err != nil {
		t.Fatal(err)
	}
	return s
}

// A team's member reads every about of the team's store and writes its
// own, the team's and its ws: only: on 10 November 2026 each QA bot had a
// store to itself, and qa-jefe's weekly plan never saw the charters
// qa-explorador recorded.
func TestTeamMemberReadsItsTeamAndWritesItsOwnAndTheTeams(t *testing.T) {
	s := teamSession(t, true)
	state := s.Export()
	member := "project:qa." + filepath.Base(string(state.Workspace))
	own := "ws:" + string(state.ID)
	recorder := &rememberRecorder{answer: map[string]any{"accepted": true}}
	recall := &recallStub{texts: map[string]string{"project:qa.qa-explorador": "- charter kmp/3 ran"}}
	u := HostToolUseCase{Memory: recorder, Recall: recall, Labels: &contextLabels{labels: map[domain.SessionID]domain.SessionLabel{}}}
	remember := func(about string) domain.ToolOutcome {
		arguments := `{"kind":"observation","text":"x","evidence":"e"}`
		if about != "" {
			arguments = `{"kind":"observation","text":"x","evidence":"e","about":"` + about + `"}`
		}
		outcome, err := u.Execute(context.Background(), s, rememberIdentity(t), mustObject(t, arguments))
		if err != nil {
			t.Fatal(err)
		}
		return outcome
	}
	for _, about := range []string{member, "project:qa", own} {
		if outcome := remember(about); outcome.IsError {
			t.Fatalf("write to %s refused: %s", about, outcome.Content)
		}
	}
	// A write that names no about lands in the member's own.
	if outcome := remember(""); outcome.IsError || recorder.requests[len(recorder.requests)-1].About != member {
		t.Fatalf("default about: %+v %+v", outcome, recorder.requests[len(recorder.requests)-1])
	}
	writes := len(recorder.requests)
	for _, about := range []string{"project:qa.qa-explorador", "project:dev", "project:" + filepath.Base(string(state.Workspace)), "user:gx10a"} {
		outcome := remember(about)
		if !outcome.IsError || !strings.Contains(string(outcome.Content), `"error":"leave about out to write under `+member+", or name one of "+member+", project:qa, "+own+":") || !strings.Contains(string(outcome.Content), about+" is a teammate's or another's") {
			t.Fatalf("write to %s: %+v", about, outcome)
		}
	}
	if len(recorder.requests) != writes {
		t.Fatalf("KMP took a write to another's about: %d", len(recorder.requests)-writes)
	}
	// A teammate's about is read as any in the store.
	outcome, err := u.Execute(context.Background(), s, recallIdentity(t), mustObject(t, `{"about":"project:qa.qa-explorador","mode":"wake"}`))
	if err != nil || outcome.IsError || !strings.Contains(string(outcome.Content), "charter kmp/3 ran") {
		t.Fatalf("read of a teammate's about: %+v %v", outcome, err)
	}
	// The model is told the team's about beside its own.
	value := sessionContextValue(s, domain.SessionLabel{})
	if value.About != member || value.TeamAbout != "project:qa" {
		t.Fatalf("session context %+v", value)
	}
	if plain := sessionContextValue(boundSession(t, true), domain.SessionLabel{}); plain.TeamAbout != "" {
		t.Fatalf("a bot with no team is told of one: %+v", plain)
	}
}

// The engine's own write, called by the model, is held to the same
// abouts in a team's store; in a store the bot has to itself it goes as
// it came.
func TestTeamMemberCannotWriteATeammatesAboutThroughTheEngine(t *testing.T) {
	s := teamSession(t, true)
	state := s.Export()
	member := "project:qa." + filepath.Base(string(state.Workspace))
	tool := func(name string) domain.AvailableTool {
		ref := root.PluginRef{PluginID: "kmp", ToolName: root.PluginToolName(name)}
		identity, _ := domain.NewPluginToolIdentity(ref)
		return domain.AvailableTool{Identity: identity, Surface: engineSurface(ref)}
	}
	write := tool("kmp_write_memory")
	for _, arguments := range []string{
		`{"about":"` + member + `","memories":[]}`,
		`{"about":"project:qa","memories":[]}`,
		`{"about":"ws:` + string(state.ID) + `","memories":[]}`,
		`{"continuation":"c_1"}`,
	} {
		if err := checkMemoryWrite(s, write, mustObject(t, arguments)); err != nil {
			t.Fatalf("%s refused: %v", arguments, err)
		}
	}
	for arguments, want := range map[string]string{
		`{"about":"project:qa.qa-explorador","memories":[]}`: "project:qa.qa-explorador is a teammate's or another's",
		`{"about":"project:dev","memories":[]}`:              "project:dev is a teammate's or another's",
		`{"memories":[]}`:                                    "name about as one of project:qa.",
		`{"about":7}`:                                        "not an object with a text about",
	} {
		if err := checkMemoryWrite(s, write, mustObject(t, arguments)); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s: %v", arguments, err)
		}
	}
	// Reads and other tools are not the write.
	if err := checkMemoryWrite(s, tool("kmp_ask"), mustObject(t, `{"about":"project:qa.qa-explorador"}`)); err != nil {
		t.Fatal(err)
	}
	if err := checkMemoryWrite(s, turnTools()[0], mustObject(t, `{"about":"project:dev"}`)); err != nil {
		t.Fatal(err)
	}
	// Without a team the store is the bot's alone.
	if err := checkMemoryWrite(boundSession(t, true), write, mustObject(t, `{"about":"project:anything","memories":[]}`)); err != nil {
		t.Fatal(err)
	}
}

// A member's first prompt wakes its own about and the team's, and each
// member registers its own agent in the team's store, so the store says
// which bot wrote a memory.
func TestTeamMemberWakesTheTeamAndRegistersItsOwnAgent(t *testing.T) {
	s := teamSession(t, false)
	state := s.Export()
	name := filepath.Base(string(state.Workspace))
	recall := &recallStub{texts: map[string]string{"project:qa": "- the weekly plan covers kmp and made"}}
	c := recallCeremony(t, recall)
	guide, agents := &guideStub{}, &agentsStub{agents: map[string]string{}}
	c.Identity = identityFor(guide, agents)
	c.Identity.Recall = recall
	c.Store = &listingStore{}
	note := c.Recall(context.Background(), &s, "plan the week")
	var abouts []string
	for _, request := range recall.requests {
		abouts = append(abouts, request.About)
	}
	if got := strings.Join(abouts, " "); got != "project:qa."+name+" project:qa" {
		t.Fatalf("woke %q", got)
	}
	if !strings.Contains(note, "Memory of project:qa:\n- the weekly plan covers kmp and made") {
		t.Fatalf("note %q", note)
	}
	key := "axlr:install-1:" + name
	if guide.calls[0] != "register "+key || agents.agents["/repo/.kernel#"+name] != "agent_"+key {
		t.Fatalf("calls %v agents %v", guide.calls, agents.agents)
	}
	if instance, _ := s.Instance(); instance.MemoryAgent != domain.MemoryAgentID("agent_"+key) {
		t.Fatalf("instance %+v", instance)
	}
	// A bot with no team keeps the store's one agent.
	alone := boundSession(t, false)
	guide.calls = nil
	c.Recall(context.Background(), &alone, "plan the week")
	if guide.calls[0] != "register axlr:install-1" || agents.agents["/repo/.kernel"] != "agent_axlr:install-1" {
		t.Fatalf("calls %v agents %v", guide.calls, agents.agents)
	}
}
