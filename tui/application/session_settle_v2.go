package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// settleV2 is settle under axlr-v2 (judge_policy_v2.go). It keeps settle's
// guards, timeout, key, context, evidence lines and routing by kind, and
// changes what Jev reads and how its answers apply: the whole request,
// answer and tool results of the turn instead of the record's excerpts;
// the lab's questions with positional IDs; every answer validated before
// it is used; the choice applied as the lab's evaluate applies it, without
// the legacy probability floors. Any failure of the gate, the sentence or
// the kind records nothing; supersede is asked only when
// JudgeDecisions.Supersede allows it, and its failure or abstention
// records the new memory without a link. Nothing falls back to the legacy
// questions.
func (c *SessionCeremony) settleV2(ctx context.Context, s domain.Session, instance domain.SessionInstance, record TurnRecord, stepRef string) {
	actions := record.Evidence(stepRef)[1:]
	if len(actions) > settleActionLines {
		actions = actions[:settleActionLines]
	}
	evidence, err := JudgeTurnEvidenceOf(s.Messages(), record.PromptIndex, false)
	if err != nil {
		c.log("settle turn %d (%s): %v", record.Turn, JudgeV2Version, err)
		return
	}
	turn := evidence.Dossier()
	gate, _, err := c.askV2(ctx, JudgeFamilyGate, turn)
	if err != nil {
		c.log("settle turn %d (%s): gate: %v", record.Turn, JudgeV2Version, err)
		return
	}
	if *gate.Yes < settleGateThreshold {
		c.log("settle turn %d (%s): nothing durable (p=%.2f)", record.Turn, JudgeV2Version, *gate.Yes)
		return
	}
	// The sentences come from the whole answer, not the record's excerpt;
	// settleSentences keeps its own bounds (at most settleCandidates).
	turn.Sentences = settleSentences(evidence.Answer)
	sentence, compiled, err := c.askV2(ctx, JudgeFamilySentence, turn)
	if errors.Is(err, ErrJudgeNoCandidates) {
		c.log("settle turn %d (%s): Jev saw something durable (p=%.2f) but the answer has no sentence to keep", record.Turn, JudgeV2Version, *gate.Yes)
		return
	}
	if err != nil {
		c.log("settle turn %d (%s): sentence: %v", record.Turn, JudgeV2Version, err)
		return
	}
	text, abstained, err := compiled.Choice(sentence)
	if err != nil || abstained {
		if err != nil {
			c.log("settle turn %d (%s): sentence: %v", record.Turn, JudgeV2Version, err)
		}
		return
	}
	kind, compiled, err := c.askV2(ctx, JudgeFamilyKind, JudgeDossier{Prompt: evidence.PersonRequest, Answer: text})
	if err != nil {
		c.log("settle turn %d (%s): kind: %v", record.Turn, JudgeV2Version, err)
		return
	}
	stateOf := s.Export()
	project := c.about(ctx, stateOf)
	about := project
	if (kind.Choice == "preference" || kind.Choice == "feedback") && c.Identity != nil && c.Identity.UserAbout != "" {
		about = c.Identity.UserAbout
	}
	// The sentence was read whole for Jev; what the store keeps is
	// redacted, as the record's answer is.
	entry := SettledEntry{About: about, Kind: kind.Choice, Text: Redact(text), SummaryEN: settledSummaryEN(kind.Choice, record),
		Evidence: append([]string{stepRef, fmt.Sprintf("Jev (%s) judged the turn durable with p=%.2f", JudgeV2Version, *gate.Yes)}, actions...), Key: fmt.Sprintf("axlr:settled:%s:%d:1", stateOf.ID, record.PromptIndex), ContextID: instance.MemoryContext, Probability: *gate.Yes,
		Labels: map[string][]string{"session": {string(stateOf.ID)}, "ws": {string(stateOf.Workspace)}, "judged_by": {"jev"}, "judge_policy": {JudgeV2Version}, "model": {record.Model}, "mode": {record.Mode}}}
	if link := c.supersedesV2(ctx, instance, entry, project, actions, stepRef); link != nil {
		entry.Links = []MemoryLink{*link}
	}
	ref, err := c.Journal.RecordSettled(ctx, entry)
	if err != nil {
		c.log("settle turn %d: %v", record.Turn, err)
		return
	}
	c.log("turn %d settled a %s in %s: %s", record.Turn, entry.Kind, entry.About, ref)
}

// supersedesV2 asks, when the policy allows it, which recalled memory the
// settled entry replaces in full, and returns the link to write. It asks
// nothing when the switch is off, when the entry goes to another about
// than the project's (the recall exposes project memories), or when a
// recalled line may be cut: a clause lost to the cut cannot be shown to be
// carried forward. The link's ref is the exact ref offered, never an ID.
func (c *SessionCeremony) supersedesV2(ctx context.Context, instance domain.SessionInstance, entry SettledEntry, project string, actions []string, stepRef string) *MemoryLink {
	if !c.Decisions.Supersede || len(instance.RecalledMemories) == 0 {
		return nil
	}
	if entry.About != project {
		c.log("settle (%s): supersede not asked: the memory goes to %s, the recalled ones are %s's", JudgeV2Version, entry.About, project)
		return nil
	}
	for _, line := range instance.RecalledMemories {
		if strings.HasSuffix(line, "…") {
			c.log("settle (%s): supersede not asked: a recalled memory is cut, its whole text is unknown", JudgeV2Version)
			return nil
		}
	}
	verdict, compiled, err := c.askV2(ctx, JudgeFamilySupersede, JudgeDossier{NewMemory: entry.Text, EarlierMemories: instance.RecalledMemories})
	if err != nil {
		c.log("settle (%s): supersede: %v", JudgeV2Version, err)
		return nil
	}
	ref, abstained, err := compiled.Choice(verdict)
	if err != nil || abstained {
		if err != nil {
			c.log("settle (%s): supersede: %v", JudgeV2Version, err)
		}
		return nil
	}
	evidence := stepRef
	if len(actions) > 0 {
		evidence = actions[0]
	}
	return &MemoryLink{Ref: ref, Rel: "supersedes", Why: fmt.Sprintf("Jev (%s) chose it as fully replaced by the turn's settled memory (p=%.2f)", JudgeV2Version, verdict.Probabilities[verdict.Choice]), Evidence: evidence}
}

// askV2 compiles one axlr-v2 decision, asks Jev and validates the answer
// against the question sent. A compilation error asks nothing.
func (c *SessionCeremony) askV2(ctx context.Context, family string, dossier JudgeDossier) (JudgementVerdict, CompiledJudgeQuestion, error) {
	compiled, err := CompileJudgeV2(family, dossier)
	if err != nil {
		return JudgementVerdict{}, compiled, err
	}
	verdict, err := c.Judge.Judge(ctx, compiled.Question)
	if err != nil {
		return JudgementVerdict{}, compiled, err
	}
	if err := ValidateJudgementVerdict(compiled.Question, verdict, c.Decisions.Model); err != nil {
		return JudgementVerdict{}, compiled, err
	}
	return verdict, compiled, nil
}
