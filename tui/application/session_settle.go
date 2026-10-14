package application

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// Jev decides what a turn settles (decisions D5 and F5 of the lifecycle
// integration design). TypeSafe Jev answers only yes/no and choice
// questions, so it selects rather than writes: a gate on the turn's record,
// the sentence of the answer that states what was settled, its kind, and
// the earlier memory it replaces, if any. The console writes the result
// to the project's about (or the person's for a preference or feedback),
// with the MADE step and the actions as evidence and no card. Where Jev
// is not configured the model records itself and the memory reminder
// stands in.

// SettledEntry is one memory Jev selected from a turn.
type SettledEntry struct {
	About, Kind, Text string
	// SummaryEN frames the quoted text in English without translating it
	// (decision K1): a strict store needs an English summary.
	SummaryEN string
	Evidence  []string
	Labels    map[string][]string
	Key       string
	ContextID domain.MemoryContextID
	Links     []MemoryLink
	// Probability is Jev's probability for the gate, kept as evidence.
	Probability float64
}

// Settlement thresholds and bounds.
const (
	settleGateThreshold      = 0.5
	settleSentenceThreshold  = 0.4
	settleSupersedeThreshold = 0.5
	settleTimeout            = 20 * time.Second
	settleCandidates         = 6
	settlePromptBytes        = 2000
	settleAnswerBytes        = 4000
	settleActionLines        = 40
	settleSentenceMin        = 20
	// settleSentenceMax is in bytes: the answers of a model in Spanish
	// carry sentences of several hundred bytes, which Jev sees bounded
	// (optionLabels) and the store keeps whole.
	settleSentenceMax = 600
)

// settleKinds are the kinds Jev chooses among, with what each means.
var settleKinds = []string{"decision", "constraint", "success_path", "error_path", "observation", "preference", "feedback"}

// settles reports whether Jev settles turns in this console.
func (c *SessionCeremony) settles() bool {
	return c != nil && c.Judge != nil && c.Journal != nil
}

// settle asks Jev what the turn settled and records it. It runs after the
// turn's record, only for a turn with results and no memory write of the
// model's, under one timeout, and never fails the turn.
func (c *SessionCeremony) settle(ctx context.Context, s domain.Session, instance domain.SessionInstance, record TurnRecord, stepRef string) {
	if !c.settles() || !record.HasResults() || len(record.Memories) > 0 || strings.TrimSpace(record.Answer) == "" {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, settleTimeout)
	defer cancel()
	actions := record.Evidence(stepRef)[1:]
	if len(actions) > settleActionLines {
		actions = actions[:settleActionLines]
	}
	state, err := json.Marshal(map[string]any{"prompt": cut(record.Prompt, settlePromptBytes), "answer": cut(record.Answer, settleAnswerBytes), "actions": actions, "files_written": record.Files, "commands_run": record.Commands})
	if err != nil {
		return
	}
	gate, err := c.Judge.Judge(ctx, JudgementQuestion{State: string(state), Question: "Did this turn settle a decision, a constraint, a fix or an outcome worth reusing later in this project? Judge only from the prompt, the answer and the actions given: a greeting, a question answered from reading, or work that changed nothing durable settles nothing."})
	if err != nil {
		c.log("settle turn %d: %v", record.Turn, err)
		return
	}
	if gate.Yes == nil || *gate.Yes < settleGateThreshold {
		// The log says why nothing was recorded, as it does for a record.
		if gate.Yes != nil {
			c.log("settle turn %d: nothing durable (p=%.2f)", record.Turn, *gate.Yes)
		}
		return
	}
	candidates := settleSentences(record.Answer)
	if len(candidates) == 0 {
		c.log("settle turn %d: Jev saw something durable (p=%.2f) but the answer has no sentence to keep", record.Turn, *gate.Yes)
		return
	}
	none := "none of them"
	labels, sentences := optionLabels(candidates)
	choice, err := c.Judge.Judge(ctx, JudgementQuestion{State: string(state), Question: "Which sentence of the answer states what this turn settled: the decision, constraint, fix or outcome worth reusing? Pick \"none of them\" when no sentence does.", Options: append(labels, none)})
	if err != nil {
		c.log("settle turn %d: %v", record.Turn, err)
		return
	}
	text := sentences[choice.Choice]
	if text == "" || choice.Choice == none || choice.Probabilities[choice.Choice] < settleSentenceThreshold {
		return
	}
	kind, err := c.Judge.Judge(ctx, JudgementQuestion{State: text, Question: "Which kind of memory is this sentence: decision (a choice made), constraint (a rule that holds), success_path (what worked), error_path (what failed and why), observation (a fact seen), preference (how the person wants things done), or feedback (a correction the person gave)?", Options: settleKinds})
	if err != nil {
		c.log("settle turn %d: %v", record.Turn, err)
		return
	}
	if kind.Choice == "" {
		kind.Choice = "observation"
	}
	stateOf := s.Export()
	about := c.about(ctx, stateOf)
	if (kind.Choice == "preference" || kind.Choice == "feedback") && c.Identity != nil && c.Identity.UserAbout != "" {
		about = c.Identity.UserAbout
	}
	entry := SettledEntry{About: about, Kind: kind.Choice, Text: text, SummaryEN: settledSummaryEN(kind.Choice, record),
		Evidence: append([]string{stepRef, fmt.Sprintf("Jev judged the turn durable with p=%.2f", *gate.Yes)}, actions...), Key: fmt.Sprintf("axlr:settled:%s:%d:1", stateOf.ID, record.PromptIndex), ContextID: instance.MemoryContext, Probability: *gate.Yes,
		Labels: map[string][]string{"session": {string(stateOf.ID)}, "ws": {string(stateOf.Workspace)}, "judged_by": {"jev"}, "model": {record.Model}, "mode": {record.Mode}}}
	if link := c.supersedes(ctx, instance, text, actions); link != nil {
		entry.Links = []MemoryLink{*link}
	}
	ref, err := c.Journal.RecordSettled(ctx, entry)
	if err != nil {
		c.log("settle turn %d: %v", record.Turn, err)
		return
	}
	c.log("turn %d settled a %s in %s: %s", record.Turn, entry.Kind, entry.About, ref)
}

// settledSummaryEN is the English search summary of a settled memory. It
// carries what the console knows in English (the kind, the turn, the
// model and the files written) and not the sentence: a strict store
// refuses a summary_en that leans to the sentence's language, however it
// is framed (seen on 13 October 2026 with a Spanish answer).
func settledSummaryEN(kind string, record TurnRecord) string {
	summary := fmt.Sprintf("%s settled in turn %d by %s (%s mode)", kind, record.Turn, record.Model, record.Mode)
	if len(record.Files) > 0 {
		summary += "; files written: " + strings.Join(record.Files, ", ")
	}
	if len(record.Commands) > 0 {
		summary += fmt.Sprintf("; %s run", plural(len(record.Commands), "command"))
	}
	return summary
}

// supersedes asks Jev whether the settled text replaces or contradicts one
// of the memories the recall exposed, and returns the link to write.
func (c *SessionCeremony) supersedes(ctx context.Context, instance domain.SessionInstance, text string, actions []string) *MemoryLink {
	if len(instance.RecalledMemories) == 0 {
		return nil
	}
	refs := map[string]string{}
	var options []string
	for _, line := range instance.RecalledMemories {
		ref, _, found := strings.Cut(line, " ")
		if !found || ref == "" || refs[ref] != "" {
			continue
		}
		refs[ref] = line
		options = append(options, ref)
	}
	if len(options) == 0 {
		return nil
	}
	sort.Strings(options)
	none := "none"
	labels, byLabel := optionLabels(options)
	state, _ := json.Marshal(map[string]any{"new_memory": text, "earlier_memories": instance.RecalledMemories})
	verdict, err := c.Judge.Judge(ctx, JudgementQuestion{State: string(state), Question: "Does new_memory replace or contradict one of earlier_memories (each line is its ref, then its text)? Pick the ref of the memory it replaces, or none.", Options: append(labels, none)})
	if err != nil || verdict.Choice == none || byLabel[verdict.Choice] == "" || verdict.Probabilities[verdict.Choice] < settleSupersedeThreshold {
		return nil
	}
	evidence := ""
	if len(actions) > 0 {
		evidence = actions[0]
	}
	return &MemoryLink{Ref: byLabel[verdict.Choice], Rel: "supersedes", Why: fmt.Sprintf("Jev judged (p=%.2f) that the turn's result replaces it", verdict.Probabilities[verdict.Choice]), Evidence: evidence}
}

// optionLabels gives Jev one label per text, bounded to what a judgement
// accepts (MaxJudgementOptionBytes; a sentence of the answer may be
// longer, seen with glm-5.3-flash on 13 October 2026), and maps each label
// back to its whole text, which is what the console stores.
func optionLabels(texts []string) ([]string, map[string]string) {
	byLabel := map[string]string{}
	var labels []string
	for _, text := range texts {
		label := cut(text, MaxJudgementOptionBytes-len("…"))
		if _, taken := byLabel[label]; taken {
			continue
		}
		byLabel[label] = text
		labels = append(labels, label)
	}
	return labels, byLabel
}

// settleSentences splits an answer into candidate sentences: a sentence
// ends at ".", "!", "?" or ";" followed by a space or a line end (so
// hola.txt stays whole); questions, list markers, short and long
// sentences and repeats are left out, at most settleCandidates.
func settleSentences(answer string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(sentence string, question bool) bool {
		sentence = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(sentence), "-*•0123456789) "))
		sentence = strings.TrimFunc(sentence, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '`' && r != '"' && r != '\''
		})
		if question || strings.HasPrefix(sentence, "¿") || len(sentence) < settleSentenceMin || len(sentence) > settleSentenceMax || seen[sentence] {
			return false
		}
		seen[sentence] = true
		out = append(out, sentence)
		return len(out) == settleCandidates
	}
	for _, line := range strings.Split(answer, "\n") {
		runes := []rune(line)
		from := 0
		for i, r := range runes {
			terminal := r == '.' || r == '!' || r == '?' || r == ';'
			if !terminal || (i+1 < len(runes) && !unicode.IsSpace(runes[i+1])) {
				continue
			}
			if add(string(runes[from:i]), r == '?') {
				return out
			}
			from = i + 1
		}
		if from < len(runes) && add(string(runes[from:]), false) {
			return out
		}
	}
	return out
}
