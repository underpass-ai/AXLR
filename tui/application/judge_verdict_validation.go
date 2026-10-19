package application

import (
	"errors"
	"fmt"
	"math"
)

// Tolerances of the lab's strict validator (transport.validate at
// JudgeV2Oracle): a choice's probabilities sum to 1 within judgeMassSlack,
// and the chosen option is the most probable within judgeChoiceSlack.
const (
	judgeMassSlack   = 0.02
	judgeChoiceSlack = 0.001
)

// ValidateJudgementVerdict checks an answer against the question sent
// before axlr-v2 applies it, as the lab's strict validator does: the model
// asked for; a yes/no answer with a finite probability in [0, 1] and no
// choice; a choice among the options offered with one probability per
// option and no other, each finite in [0, 1], summing to 1 and highest on
// the choice, with a confidence in [0, 1]. Confidence is reported, never
// read as the probability of being right. The adapter checks the reply's
// shape (one answer under q, its type, no null probability); legacy and
// axlr_judge do not call this.
func ValidateJudgementVerdict(question JudgementQuestion, verdict JudgementVerdict, model string) error {
	if model != "" && verdict.Model != model {
		return fmt.Errorf("Jev answered as %q, not the pinned %q", verdict.Model, model)
	}
	if len(question.Options) == 0 {
		if verdict.Yes == nil || !unitInterval(*verdict.Yes) {
			return errors.New("Jev's yes/no answer has no probability in [0, 1]")
		}
		if verdict.Choice != "" || verdict.Probabilities != nil || verdict.Confidence != nil {
			return errors.New("Jev answered a yes/no question with a choice")
		}
		return nil
	}
	if verdict.Yes != nil {
		return errors.New("Jev answered a choice with a yes/no probability")
	}
	if verdict.Confidence == nil || !unitInterval(*verdict.Confidence) {
		return errors.New("Jev's choice has no confidence in [0, 1]")
	}
	if len(verdict.Probabilities) != len(question.Options) {
		return errors.New("Jev's probabilities do not match the options offered")
	}
	offered, total, highest := false, 0.0, 0.0
	for _, option := range question.Options {
		p, ok := verdict.Probabilities[option]
		if !ok || !unitInterval(p) {
			return errors.New("Jev's probabilities do not match the options offered")
		}
		offered = offered || option == verdict.Choice
		total += p
		highest = math.Max(highest, p)
	}
	if !offered {
		return fmt.Errorf("Jev chose %q, which was not offered", verdict.Choice)
	}
	if math.Abs(total-1) > judgeMassSlack {
		return fmt.Errorf("Jev's probabilities sum to %.3f, not 1", total)
	}
	if verdict.Probabilities[verdict.Choice]+judgeChoiceSlack < highest {
		return errors.New("Jev's choice is not its most probable option")
	}
	return nil
}

func unitInterval(p float64) bool { return !math.IsNaN(p) && !math.IsInf(p, 0) && p >= 0 && p <= 1 }
