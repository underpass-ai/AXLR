package application

import (
	"context"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// scriptedJudge answers each question by what it asks: the gate with a
// probability, the choices with a pick and its probability.
type scriptedJudge struct {
	gate      float64
	sentence  string
	sentenceP float64
	kind      string
	replaces  string
	replacesP float64
	questions []JudgementQuestion
}

func (j *scriptedJudge) Judge(_ context.Context, q JudgementQuestion) (JudgementVerdict, error) {
	j.questions = append(j.questions, q)
	switch {
	case len(q.Options) == 0:
		p := j.gate
		return JudgementVerdict{Yes: &p}, nil
	case strings.Contains(q.Question, "Which sentence"):
		return JudgementVerdict{Choice: j.sentence, Probabilities: map[string]float64{j.sentence: j.sentenceP}}, nil
	case strings.Contains(q.Question, "Which kind"):
		return JudgementVerdict{Choice: j.kind, Probabilities: map[string]float64{j.kind: 0.9}}, nil
	case strings.Contains(q.Question, "replace or contradict"):
		return JudgementVerdict{Choice: j.replaces, Probabilities: map[string]float64{j.replaces: j.replacesP}}, nil
	}
	return JudgementVerdict{}, nil
}

func settledTurn(t *testing.T, c *SessionCeremony, recalled []string) (*journalStub, domain.Session) {
	t.Helper()
	return settledTurnAnswer(t, c, recalled, settleAnswer)
}

// settledTurnAnswer is settledTurn with the answer the turn ends on.
func settledTurnAnswer(t *testing.T, c *SessionCeremony, recalled []string, answer string) (*journalStub, domain.Session) {
	t.Helper()
	s := turnSession(t)
	if err := s.BeginTurn("añade hola.txt y comprueba", turnTools()); err != nil {
		t.Fatal(err)
	}
	c.BeginTurn(context.Background(), &s)
	if len(recalled) > 0 {
		instance, _ := s.Instance()
		instance.RecalledMemories = recalled
		if err := s.SetInstance(instance); err != nil {
			t.Fatal(err)
		}
	}
	settleTurnOn(t, &s, answer)
	if err := c.EndTurn(context.Background(), &s); err != nil {
		t.Fatal(err)
	}
	return c.Journal.(*journalStub), s
}

const settleAnswer = "Hecho. Las fechas del parser se guardan en UTC, como pide la RFC 3339. Los tests pasan."

// settleTurnOn records a turn that wrote a file and ran a check, recorded
// no memory itself, and answered with the decision it took.
func settleTurnOn(t *testing.T, s *domain.Session, answer string) {
	t.Helper()
	calls := []root.ToolCall{
		{ID: "c1", Name: "local_write", Arguments: mustObject(t, `{"path":"parser.go","content":"x","mode":"create","why":"keep dates in UTC"}`)},
		{ID: "c2", Name: "local_exec", Arguments: mustObject(t, `{"program":"go","args":["test","./parser"],"why":"check the parser"}`)},
	}
	if err := s.CompleteAssistant(assistant("", calls...)); err != nil {
		t.Fatal(err)
	}
	outcomes := []domain.ToolOutcome{
		{Content: `{"protocol_version":1,"tool":"write","status":"completed","output":{"written_bytes":1},"error":null}`},
		{Content: `{"protocol_version":1,"tool":"exec","status":"completed","output":{"exit_code":0,"stdout":"ok","stderr":"","captured_bytes":2},"error":null}`},
	}
	for i, outcome := range outcomes {
		if err := s.RecordToolOutcome(calls[i].ID, domain.DecisionAutoApprove, outcome); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.CompleteAssistant(assistant(root.Text(answer))); err != nil {
		t.Fatal(err)
	}
}

// A sentence longer than one judgement's option reaches Jev as a bounded
// label and is stored whole (seen with glm-5.3-flash: TypeSafe refused
// "each option must be 1 to 200 bytes" and the turn settled nothing).
func TestJevSettlementBoundsTheOptionsAndStoresTheWholeSentence(t *testing.T) {
	engine := &fakeEngine{}
	c, _, _, _ := sessionCeremonyFor(t, engine)
	long := "Todos los ficheros del proyecto usarán UTF-8 con finales de línea LF porque " + strings.Repeat("la codificación única evita ambigüedades, ", 3) + "y los CRLF de Windows ensucian los diffs"
	label := cut(long, MaxJudgementOptionBytes-len("…"))
	if len(long) <= MaxJudgementOptionBytes || len(long) > settleSentenceMax || len(label) > MaxJudgementOptionBytes {
		t.Fatalf("sentence %d bytes, label %d bytes", len(long), len(label))
	}
	judge := &scriptedJudge{gate: 0.8, sentence: label, sentenceP: 0.7, kind: "constraint"}
	c.Judge = judge
	journal, _ := settledTurnAnswer(t, c, nil, "Hecho. "+long+". Los tests pasan.")
	if len(journal.settled) != 1 || journal.settled[0].Text != long || journal.settled[0].Kind != "constraint" {
		t.Fatalf("settled %+v after %d questions", journal.settled, len(judge.questions))
	}
	if judge.questions[1].Options[0] != label {
		t.Fatalf("options %q", judge.questions[1].Options)
	}
	for _, option := range judge.questions[1].Options {
		if len(option) > MaxJudgementOptionBytes {
			t.Fatalf("option over the bound: %q", option)
		}
	}
}

// Jev gates the turn, picks the sentence that states what it settled and
// its kind, and names the memory it replaces; the console writes it to
// the project about with the step and the actions as evidence.
func TestJevSettlesATurnIntoTheProjectAbout(t *testing.T) {
	engine := &fakeEngine{}
	c, _, _, _ := sessionCeremonyFor(t, engine)
	judge := &scriptedJudge{gate: 0.8, sentence: "Las fechas del parser se guardan en UTC, como pide la RFC 3339", sentenceP: 0.7, kind: "decision", replaces: "project:x:entry:decision:old", replacesP: 0.6}
	c.Judge = judge
	journal, s := settledTurn(t, c, []string{"project:x:entry:decision:old (decision): las fechas se guardan en hora local", "project:x:entry:constraint:k (constraint): otra"})
	if len(journal.settled) != 1 {
		t.Fatalf("settled %+v questions %d", journal.settled, len(judge.questions))
	}
	entry := journal.settled[0]
	id := string(s.ID())
	if entry.Kind != "decision" || entry.Text != "Las fechas del parser se guardan en UTC, como pide la RFC 3339" || entry.Key != "axlr:settled:"+id+":0:1" || entry.Labels["judged_by"][0] != "jev" || !strings.HasPrefix(entry.SummaryEN, "decision settled in turn 1") || !strings.Contains(entry.SummaryEN, entry.Text) || !strings.HasPrefix(entry.Evidence[0], "MADE axlr-session-") || !strings.Contains(entry.Evidence[1], "p=0.80") || entry.Probability != 0.8 {
		t.Fatalf("entry %+v", entry)
	}
	if entry.About != "project:"+defaultAboutName(t, s) {
		t.Fatalf("about %q", entry.About)
	}
	if len(entry.Links) != 1 || entry.Links[0].Ref != "project:x:entry:decision:old" || entry.Links[0].Rel != "supersedes" || !strings.Contains(entry.Links[0].Why, "p=0.60") {
		t.Fatalf("links %+v", entry.Links)
	}
	if len(judge.questions) != 4 || len(judge.questions[1].Options) < 2 || judge.questions[1].Options[len(judge.questions[1].Options)-1] != "none of them" || judge.questions[3].Options[0] != "project:x:entry:constraint:k" {
		t.Fatalf("questions %+v", judge.questions)
	}
}

func defaultAboutName(t *testing.T, s domain.Session) string {
	t.Helper()
	about := defaultAbout(s.Export())
	return strings.TrimPrefix(about, "project:")
}

// A preference goes to the person's about; a turn Jev finds nothing
// durable in, or whose chosen sentence is doubtful, writes nothing.
func TestJevSettlementRoutesAndDeclines(t *testing.T) {
	engine := &fakeEngine{}
	c, _, _, _ := sessionCeremonyFor(t, engine)
	c.Identity = &KMPIdentity{Store: "/s", RegistrationKey: "k", Agents: &agentsStub{agents: map[string]string{}}, Guide: &guideStub{}, UserAbout: "user:tirso"}
	c.Judge = &scriptedJudge{gate: 0.9, sentence: "Las fechas del parser se guardan en UTC, como pide la RFC 3339", sentenceP: 0.8, kind: "preference", replaces: "none", replacesP: 0.9}
	journal, _ := settledTurn(t, c, nil)
	if len(journal.settled) != 1 || journal.settled[0].About != "user:tirso" || len(journal.settled[0].Links) != 0 {
		t.Fatalf("%+v", journal.settled)
	}
	for name, judge := range map[string]*scriptedJudge{
		"nothing durable": {gate: 0.2},
		"no sentence":     {gate: 0.9, sentence: "none of them", sentenceP: 0.9},
		"doubtful":        {gate: 0.9, sentence: "Las fechas del parser se guardan en UTC, como pide la RFC 3339", sentenceP: 0.2, kind: "decision"},
	} {
		c, _, _, _ := sessionCeremonyFor(t, &fakeEngine{})
		c.Judge = judge
		if journal, _ := settledTurn(t, c, nil); len(journal.settled) != 0 {
			t.Fatalf("%s: %+v", name, journal.settled)
		}
	}
	// Without Jev nothing is asked and the reminder stands.
	c, _, _, _ = sessionCeremonyFor(t, &fakeEngine{})
	if c.settles() {
		t.Fatal("settles without Jev")
	}
	s := turnSession(t)
	if err := s.BeginTurn("x", memoryTools(t)); err != nil {
		t.Fatal(err)
	}
	c.Judge = &scriptedJudge{}
	if !c.settles() {
		t.Fatal("does not settle with Jev and a journal")
	}
	if reminded, err := remindMemory(context.Background(), &s, ContinueTurnUseCase{Store: &memoryStore{}, Session: c}, func(Event) error { return nil }); reminded || err != nil {
		t.Fatalf("reminded with Jev settling: %v %v", reminded, err)
	}
}

func TestSettleSentencesAndRecalledMemories(t *testing.T) {
	answer := "Hecho. Las fechas del parser se guardan en UTC, como pide la RFC 3339. ¿Quieres que lo documente?\n- Añadí hola.txt con el texto pedido.\n1. Línea corta.\nLas fechas del parser se guardan en UTC, como pide la RFC 3339."
	got := settleSentences(answer)
	if len(got) != 2 || got[0] != "Las fechas del parser se guardan en UTC, como pide la RFC 3339" || got[1] != "Añadí hola.txt con el texto pedido" {
		t.Fatalf("%q", got)
	}
	lines := recalledMemories("- project:x:entry:decision:a (decision): keep UTC\n- Partial KMP recall; do not treat it as complete.\n- project:x:entry:constraint:b (constraint): "+strings.Repeat("x", 400), []string{"project:x:entry:decision:a", "project:x:entry:constraint:b"})
	if len(lines) != 2 || lines[0] != "project:x:entry:decision:a (decision): keep UTC" || len(lines[1]) > recalledMemoryBytes+3 {
		t.Fatalf("%q", lines)
	}
}
