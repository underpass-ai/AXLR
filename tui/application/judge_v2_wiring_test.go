package application

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// v2FinalTurn records a request that read a file whose content contradicts
// the answer, and asks the final check under axlr-v2.
func v2FinalTurn(t *testing.T, judge JudgementPort, diagnostics DiagnosticPort, result string) (*domain.Session, bool, error) {
	t.Helper()
	s := turnSession(t)
	if err := s.BeginTurn("Comprueba si la migración terminó\n\n"+RecallNotePrefix+" Memory of project:x", turnTools()); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAssistant(assistant("", call("r1", "read"))); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordToolOutcome("r1", domain.DecisionApprove, domain.ToolOutcome{Content: root.Text(result)}); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAssistant(assistant("La migración terminó.")); err != nil {
		t.Fatal(err)
	}
	u := ContinueTurnUseCase{Store: &memoryStore{}, Judge: &Judge{Port: judge, FinalCheck: true, Decisions: v2Decisions()}, Diagnostics: diagnostics}
	asked, err := checkFinalAnswer(context.Background(), &s, u, ignoreEvent)
	return &s, asked, err
}

// Under axlr-v2 the final check reads the whole request, answer and tool
// result, asks the lab's question once and, below the threshold, returns
// the verdict to the model as before.
func TestJudgeV2FinalCheckReadsTheWholeTurn(t *testing.T) {
	stdout := strings.Repeat("progreso ", 80) + "ERROR: la migración se revirtió"
	result := `{"status":"completed","error":null,"output":{"exit_code":0,"stdout":"` + stdout + `"}}`
	answer := v2Yes(0.2)
	answer.family = JudgeFamilyFinal
	judge := &v2Judge{t: t, answers: []v2Answer{answer}}
	s, asked, err := v2FinalTurn(t, judge, nil, result)
	if err != nil || !asked || len(judge.questions) != 1 {
		t.Fatalf("asked=%v questions=%d err=%v", asked, len(judge.questions), err)
	}
	state := judge.questions[0].State
	if !strings.Contains(state, `"person_request":"Comprueba si la migración terminó"`) || strings.Contains(state, "Memory of project") || !strings.Contains(state, "se revirtió") || !strings.Contains(state, `"answer":"La migración terminó."`) || len(judge.questions[0].Options) != 0 {
		t.Fatalf("state %s", state)
	}
	last := s.Messages()[len(s.Messages())-1]
	if last.Role != root.RoleUser || !strings.HasPrefix(string(last.Content), jevFinalPrefix) || !strings.Contains(string(last.Content), "20%") {
		t.Fatalf("note %q", last.Content)
	}
	// The note is out: the request is not checked again.
	if err := s.CompleteAssistant(assistant("La migración se revirtió; la repito.")); err != nil {
		t.Fatal(err)
	}
	u := ContinueTurnUseCase{Store: &memoryStore{}, Judge: &Judge{Port: judge, FinalCheck: true, Decisions: v2Decisions()}}
	if again, err := checkFinalAnswer(context.Background(), s, u, ignoreEvent); again || err != nil || len(judge.questions) != 1 {
		t.Fatalf("checked twice: %v %v", again, err)
	}
}

// An invalid answer, a failed call or evidence too large to send whole
// lets the answer stand; the omission is counted.
func TestJudgeV2FinalCheckNeverBlocksOnABadJudgement(t *testing.T) {
	nan := math.NaN()
	for name, answer := range map[string]v2Answer{
		"other model":  {family: JudgeFamilyFinal, verdict: JudgementVerdict{Model: "jev-latest", Yes: yes(0.1)}},
		"no number":    {family: JudgeFamilyFinal, verdict: JudgementVerdict{Model: v2Model, Yes: &nan}},
		"a choice":     {family: JudgeFamilyFinal, verdict: v2Pick([]string{"a", "b"}, "a", 0.9)},
		"out of range": {family: JudgeFamilyFinal, verdict: JudgementVerdict{Model: v2Model, Yes: yes(1.5)}},
		"transport":    {family: JudgeFamilyFinal, err: errTransport},
	} {
		judge := &v2Judge{t: t, answers: []v2Answer{answer}}
		trace := &diagnosticCollector{}
		s, asked, err := v2FinalTurn(t, judge, trace, `{"status":"completed","error":null,"output":{}}`)
		if asked || err != nil || len(judge.questions) != 1 || s.Messages()[len(s.Messages())-1].Role != root.RoleAssistant {
			t.Fatalf("%s: asked=%v err=%v", name, asked, err)
		}
		if len(trace.events) != 1 || trace.events[0].ErrorClass != DiagnosticErrorProvider {
			t.Fatalf("%s: %+v", name, trace.events)
		}
	}
	judge := &v2Judge{t: t}
	trace := &diagnosticCollector{}
	if _, asked, err := v2FinalTurn(t, judge, trace, strings.Repeat("x", MaxJudgementStateBytes)); asked || err != nil || len(judge.questions) != 0 {
		t.Fatalf("too large: asked=%v err=%v", asked, err)
	}
	if len(trace.events) != 1 || trace.events[0].ErrorClass != DiagnosticErrorInvalidState {
		t.Fatalf("omission not counted: %+v", trace.events)
	}
}

// The final check's guards hold under axlr-v2: a request the person
// decided by denying a call, or with no calls left, is not checked.
func TestJudgeV2FinalCheckKeepsItsGuards(t *testing.T) {
	judge := &v2Judge{t: t}
	s := turnSession(t)
	if err := s.BeginTurn("vet it", turnTools()); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAssistant(assistant("", call("v", "read"))); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordToolOutcome("v", domain.DecisionDeny, domain.ToolOutcome{Content: "tool call denied by user: ya basta", IsError: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAssistant(assistant("No lo ejecuté.")); err != nil {
		t.Fatal(err)
	}
	u := ContinueTurnUseCase{Store: &memoryStore{}, Judge: &Judge{Port: judge, FinalCheck: true, Decisions: v2Decisions()}}
	if asked, err := checkFinalAnswer(context.Background(), &s, u, ignoreEvent); asked || err != nil || len(judge.questions) != 0 {
		t.Fatalf("denied: %v %v", asked, err)
	}
	u.TurnToolCalls = 1
	s2 := turnSession(t)
	if err := s2.BeginTurn("lee", turnTools()); err != nil {
		t.Fatal(err)
	}
	if err := s2.CompleteAssistant(assistant("", call("r", "read"))); err != nil {
		t.Fatal(err)
	}
	if err := s2.RecordToolOutcome("r", domain.DecisionApprove, domain.ToolOutcome{Content: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := s2.CompleteAssistant(assistant("Leído.")); err != nil {
		t.Fatal(err)
	}
	if asked, err := checkFinalAnswer(context.Background(), &s2, u, ignoreEvent); asked || err != nil || len(judge.questions) != 0 {
		t.Fatalf("no calls left: %v %v", asked, err)
	}
}

// settleV2Answers scripts gate, sentence and kind for a turn whose answer
// has n candidate sentences, the chosen one at pick (1-based).
func settleV2Answers(n, pick int, kind string) []v2Answer {
	options := append(optionIDs("s", n), "none")
	return []v2Answer{gateAnswer(0.8), choiceAnswer(JudgeFamilySentence, options, "s"+itoa(pick), 0.6), choiceAnswer(JudgeFamilyKind, settleKinds, kind, 0.7)}
}

// Under axlr-v2 the settlement reads the whole answer, so a decisive
// sentence past the record's 500-byte excerpt is a candidate, kept whole;
// supersede is not asked with the switch off, recalled memories or not.
func TestJudgeV2SettlesFromTheWholeAnswer(t *testing.T) {
	c, _, _, _ := sessionCeremonyFor(t, &fakeEngine{})
	filler := strings.Repeat("Revisé cada módulo del parser con cuidado y sin cambios. ", 10)
	decisive := "Las fechas del parser se guardan en UTC, como pide la RFC 3339"
	answer := filler + decisive + ". Secreto API_KEY=sk-abcdefghijklmnop guardado aparte."
	sentences := settleSentences(answer)
	if len(answer) < turnExcerptBytes+100 || len(sentences) != 3 || sentences[1] != decisive {
		t.Fatalf("fixture: %d bytes, %q", len(answer), sentences)
	}
	judge := &v2Judge{t: t, answers: settleV2Answers(3, 2, "decision")}
	c.Judge, c.Decisions = judge, v2Decisions()
	journal, s := settledTurnAnswer(t, c, []string{"project:x:entry:decision:old (decision): las fechas en hora local"}, answer)
	if len(journal.settled) != 1 || len(judge.questions) != 3 {
		t.Fatalf("settled %+v after %d questions", journal.settled, len(judge.questions))
	}
	entry := journal.settled[0]
	if entry.Text != decisive || entry.Kind != "decision" || len(entry.Links) != 0 || entry.Labels["judge_policy"][0] != JudgeV2Version || entry.Key != "axlr:settled:"+string(s.ID())+":0:1" || entry.About != "project:"+defaultAboutName(t, s) {
		t.Fatalf("entry %+v", entry)
	}
	if !strings.Contains(judge.questions[1].State, `"candidates":[{"id":"s1"`) || !strings.Contains(judge.questions[2].State, `"answer":"`+decisive+`"`) || !strings.Contains(judge.questions[2].State, `"person_request":"añade hola.txt y comprueba"`) {
		t.Fatalf("questions %+v", judge.questions)
	}
	if strings.Contains(judge.questions[0].State, "sk-abcdefghijklmnop") {
		t.Fatal("secret sent to Jev")
	}
}

// A secret in the chosen sentence is redacted where it is stored; Jev's
// copy is redacted too.
func TestJudgeV2StoresTheSentenceRedacted(t *testing.T) {
	c, _, _, _ := sessionCeremonyFor(t, &fakeEngine{})
	answer := "El despliegue usa la variable DEPLOY_TOKEN=abc123secret para firmar. Los tests pasan bien."
	judge := &v2Judge{t: t, answers: settleV2Answers(2, 1, "constraint")}
	c.Judge, c.Decisions = judge, v2Decisions()
	journal, _ := settledTurnAnswer(t, c, nil, answer)
	if len(journal.settled) != 1 || strings.Contains(journal.settled[0].Text, "abc123secret") || !strings.Contains(journal.settled[0].Text, "[redacted]") {
		t.Fatalf("%+v", journal.settled)
	}
}

// Any failure or negative of the gate, the sentence or the kind writes
// nothing and asks nothing further; nothing falls back to the legacy
// questions (v2Judge fails on any question it was not scripted for).
func TestJudgeV2SettlementDeclines(t *testing.T) {
	options := []string{"s1", "none"}
	bad := v2Pick(options, "s1", 0.6)
	bad.Probabilities["none"] = 0.9
	nullish := v2Pick(settleKinds, "decision", 0.7)
	delete(nullish.Probabilities, "feedback")
	for name, answers := range map[string][]v2Answer{
		"gate negative":       {gateAnswer(0.4)},
		"gate transport":      {{family: JudgeFamilyGate, err: errTransport}},
		"gate cancelled":      {{family: JudgeFamilyGate, err: context.Canceled}},
		"gate other model":    {{family: JudgeFamilyGate, verdict: JudgementVerdict{Model: "jev-1.12.0", Yes: yes(0.9)}}},
		"sentence none":       {gateAnswer(0.9), choiceAnswer(JudgeFamilySentence, options, "none", 0.7)},
		"sentence incoherent": {gateAnswer(0.9), {family: JudgeFamilySentence, verdict: bad}},
		"sentence 429":        {gateAnswer(0.9), {family: JudgeFamilySentence, err: errTransport}},
		"kind missing option": {gateAnswer(0.9), choiceAnswer(JudgeFamilySentence, options, "s1", 0.6), {family: JudgeFamilyKind, verdict: nullish}},
		"kind not offered":    {gateAnswer(0.9), choiceAnswer(JudgeFamilySentence, options, "s1", 0.6), {family: JudgeFamilyKind, verdict: v2Pick(append(append([]string(nil), settleKinds[:6]...), "fact"), "fact", 0.9)}},
	} {
		c, _, _, _ := sessionCeremonyFor(t, &fakeEngine{})
		judge := &v2Judge{t: t, answers: answers}
		c.Judge, c.Decisions = judge, v2Decisions()
		journal, _ := settledTurn(t, c, nil)
		if len(journal.settled) != 0 || len(judge.questions) != len(answers) {
			t.Fatalf("%s: settled %+v after %d questions", name, journal.settled, len(judge.questions))
		}
	}
}

// settle's guards come first under axlr-v2: a turn the model recorded a
// memory in, or without results, asks Jev nothing.
func TestJudgeV2SettlementKeepsItsGuards(t *testing.T) {
	c, _, _, _ := sessionCeremonyFor(t, &fakeEngine{})
	judge := &v2Judge{t: t}
	c.Judge, c.Decisions = judge, v2Decisions()
	s := turnSession(t)
	if err := s.BeginTurn("hola", turnTools()); err != nil {
		t.Fatal(err)
	}
	if err := s.CompleteAssistant(assistant("Hola, ¿en qué te ayudo hoy con el proyecto?")); err != nil {
		t.Fatal(err)
	}
	c.settle(context.Background(), s, domain.SessionInstance{}, BuildTurnRecord(s, 0, 1, "project:x", nil), "step")
	record := BuildTurnRecord(s, 0, 1, "project:x", nil)
	record.Actions = []TurnAction{{Tool: string(HostRememberName)}}
	record.Memories = []TurnMemory{{About: "project:x", Key: "k"}}
	c.settle(context.Background(), s, domain.SessionInstance{}, record, "step")
	if len(judge.questions) != 0 {
		t.Fatalf("asked %d", len(judge.questions))
	}
}

// With the switch on, supersede offers the recalled memories by position
// in the order received and links the exact ref; an abstention, a failure
// or an answer outside the offer records the memory without a link.
func TestJudgeV2SupersedeLinksOnlyAnOfferedRef(t *testing.T) {
	recalled := []string{"project:x:entry:constraint:z (constraint): otra cosa", "project:x:entry:decision:a (decision): las fechas se guardan en hora local"}
	supersede := func(choice string, err error) v2Answer {
		if err != nil {
			return v2Answer{family: JudgeFamilySupersede, err: err}
		}
		return choiceAnswer(JudgeFamilySupersede, []string{"m1", "m2", "none"}, choice, 0.7)
	}
	outside := supersede("m2", nil)
	outside.verdict.Choice, outside.verdict.Probabilities = "project:x:entry:decision:a", map[string]float64{"m1": 0.1, "m2": 0.2, "none": 0.7}
	for name, tc := range map[string]struct {
		answer v2Answer
		link   string
	}{
		"replaces":    {supersede("m2", nil), "project:x:entry:decision:a"},
		"none":        {supersede("none", nil), ""},
		"transport":   {supersede("", errTransport), ""},
		"not offered": {outside, ""},
	} {
		c, _, _, _ := sessionCeremonyFor(t, &fakeEngine{})
		judge := &v2Judge{t: t, answers: append(settleV2Answers(1, 1, "decision"), tc.answer)}
		c.Judge, c.Decisions = judge, v2Decisions()
		c.Decisions.Supersede = true
		journal, _ := settledTurn(t, c, recalled)
		if len(journal.settled) != 1 || len(judge.questions) != 4 {
			t.Fatalf("%s: settled %+v after %d questions", name, journal.settled, len(judge.questions))
		}
		links := journal.settled[0].Links
		if (tc.link == "") != (len(links) == 0) || (tc.link != "" && (links[0].Ref != tc.link || links[0].Rel != "supersedes" || !strings.Contains(links[0].Why, JudgeV2Version) || links[0].Evidence == "")) {
			t.Fatalf("%s: links %+v", name, links)
		}
		q := judge.questions[3]
		var state struct {
			Earlier []struct{ ID, Ref, Text string } `json:"earlier_memories"`
		}
		if err := json.Unmarshal([]byte(q.State), &state); err != nil || len(state.Earlier) != 2 || state.Earlier[0].Ref != "project:x:entry:constraint:z" || state.Earlier[1].ID != "m2" || state.Earlier[1].Text != "(decision): las fechas se guardan en hora local" {
			t.Fatalf("%s: state %s", name, q.State)
		}
	}
}

// Supersede is not asked when its switch is off, when the memory goes to
// the person's about while the recall exposed the project's, when a
// recalled line may be cut, or when the memories exceed one question;
// the new memory is recorded in every case.
func TestJudgeV2SupersedeIsNotAskedOutsideItsScope(t *testing.T) {
	many := make([]string, 16)
	for i := range many {
		many[i] = "project:x:entry:decision:" + itoa(i) + " memoria " + itoa(i)
	}
	for name, tc := range map[string]struct {
		recalled  []string
		kind      string
		supersede bool
	}{
		"switch off": {[]string{"project:x:entry:decision:a texto"}, "decision", false},
		"person":     {[]string{"project:x:entry:decision:a texto"}, "preference", true},
		"cut":        {[]string{"project:x:entry:decision:a texto", "project:x:entry:decision:b " + strings.Repeat("y", 280) + "…"}, "decision", true},
		"sixteen":    {many, "decision", true},
	} {
		c, _, _, _ := sessionCeremonyFor(t, &fakeEngine{})
		c.Identity = &MemoryIdentity{Store: "/s", RegistrationKey: "k", Agents: &agentsStub{agents: map[string]string{}}, Guide: &guideStub{}, UserAbout: "user:tirso"}
		judge := &v2Judge{t: t, answers: settleV2Answers(1, 1, tc.kind)}
		c.Judge, c.Decisions = judge, v2Decisions()
		c.Decisions.Supersede = tc.supersede
		journal, _ := settledTurn(t, c, tc.recalled)
		if len(journal.settled) != 1 || len(journal.settled[0].Links) != 0 || len(judge.questions) != 3 {
			t.Fatalf("%s: settled %+v after %d questions", name, journal.settled, len(judge.questions))
		}
		if tc.kind == "preference" && journal.settled[0].About != "user:tirso" {
			t.Fatalf("%s: about %q", name, journal.settled[0].About)
		}
	}
}

// The validator refuses every malformed distribution before anything is
// applied, with the lab's tolerances.
func TestValidateJudgementVerdict(t *testing.T) {
	choice := JudgementQuestion{State: "s", Question: "q", Options: []string{"a", "b", "none"}}
	noul := JudgementQuestion{State: "s", Question: "q"}
	good := v2Pick(choice.Options, "a", 0.6)
	if err := ValidateJudgementVerdict(choice, good, v2Model); err != nil {
		t.Fatal(err)
	}
	if err := ValidateJudgementVerdict(noul, v2Yes(0).verdict, v2Model); err != nil {
		t.Fatal(err)
	}
	edge := v2Pick(choice.Options, "a", 0.5)
	edge.Probabilities = map[string]float64{"a": 0.5, "b": 0.5009, "none": 0.0}
	if err := ValidateJudgementVerdict(choice, edge, v2Model); err != nil {
		t.Fatalf("within the choice slack: %v", err)
	}
	mutate := func(f func(*JudgementVerdict)) JudgementVerdict {
		v := v2Pick(choice.Options, "a", 0.6)
		probabilities := map[string]float64{}
		for k, p := range v.Probabilities {
			probabilities[k] = p
		}
		v.Probabilities = probabilities
		f(&v)
		return v
	}
	inf, nan := math.Inf(1), math.NaN()
	for name, verdict := range map[string]JudgementVerdict{
		"model":           mutate(func(v *JudgementVerdict) { v.Model = "jev-1.13.1" }),
		"no model":        mutate(func(v *JudgementVerdict) { v.Model = "" }),
		"missing key":     mutate(func(v *JudgementVerdict) { delete(v.Probabilities, "b") }),
		"extra key":       mutate(func(v *JudgementVerdict) { v.Probabilities["c"] = 0 }),
		"renamed key":     mutate(func(v *JudgementVerdict) { delete(v.Probabilities, "b"); v.Probabilities["c"] = 0.2 }),
		"NaN":             mutate(func(v *JudgementVerdict) { v.Probabilities["b"] = nan }),
		"Inf":             mutate(func(v *JudgementVerdict) { v.Probabilities["b"] = inf }),
		"negative":        mutate(func(v *JudgementVerdict) { v.Probabilities["b"] = -0.1; v.Probabilities["none"] = 0.5 }),
		"mass":            mutate(func(v *JudgementVerdict) { v.Probabilities["none"] = 0.3 }),
		"incoherent":      mutate(func(v *JudgementVerdict) { v.Choice = "b" }),
		"not offered":     mutate(func(v *JudgementVerdict) { v.Choice = "c" }),
		"no confidence":   mutate(func(v *JudgementVerdict) { v.Confidence = nil }),
		"confidence NaN":  mutate(func(v *JudgementVerdict) { v.Confidence = &nan }),
		"confidence 2":    mutate(func(v *JudgementVerdict) { two := 2.0; v.Confidence = &two }),
		"yes on a choice": mutate(func(v *JudgementVerdict) { v.Yes = yes(0.5) }),
	} {
		if err := ValidateJudgementVerdict(choice, verdict, v2Model); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	for name, verdict := range map[string]JudgementVerdict{
		"null":      {Model: v2Model},
		"NaN":       {Model: v2Model, Yes: &nan},
		"Inf":       {Model: v2Model, Yes: &inf},
		"above one": {Model: v2Model, Yes: yes(1.01)},
		"a choice":  {Model: v2Model, Yes: yes(0.5), Choice: "a"},
	} {
		if err := ValidateJudgementVerdict(noul, verdict, v2Model); err == nil {
			t.Errorf("yes/no %s accepted", name)
		}
	}
}

// The policy is read strictly: legacy is the default, an unknown name an
// error.
func TestParseJudgePolicy(t *testing.T) {
	for name, want := range map[string]JudgePolicy{"": JudgePolicyLegacy, "legacy": JudgePolicyLegacy, "axlr-v2": JudgePolicyAXLRv2} {
		if got, err := ParseJudgePolicy(name); got != want || err != nil {
			t.Fatalf("%q: %q %v", name, got, err)
		}
	}
	for _, name := range []string{"v2", "axlr-v5", "AXLR-V2", "legacy "} {
		if _, err := ParseJudgePolicy(name); err == nil {
			t.Fatalf("%q accepted", name)
		}
	}
}
