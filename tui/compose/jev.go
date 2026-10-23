package compose

import (
	"errors"
	"fmt"
	"time"

	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/adapters/typesafe"
	"github.com/underpass-ai/AXLR/tui/application"
)

// JevJudge builds the TypeSafe Jev judge when settings enable it; nil keeps
// Jev off and sends nothing to TypeSafe.
func JevJudge(settings *storage.JevSettings, getenv func(string) string) (*application.Judge, error) {
	if !settings.Enabled() {
		return nil, nil
	}
	policy, err := application.ParseJudgePolicy(settings.JudgePolicy)
	if err != nil {
		return nil, err
	}
	if settings.V2Supersede && policy != application.JudgePolicyAXLRv2 {
		return nil, errors.New("jev.v2_supersede needs jev.judge_policy axlr-v2")
	}
	client, err := typesafe.New(typesafe.Config{APIKey: getenv("TYPESAFE_API_KEY"), Model: settings.Model, Timeout: time.Duration(settings.TimeoutMS) * time.Millisecond})
	if err != nil {
		return nil, fmt.Errorf("jev: %w", err)
	}
	model := settings.Model
	if model == "" {
		model = typesafe.DefaultModel
	}
	decisions := application.JudgeDecisions{Policy: policy, Supersede: settings.V2Supersede, Model: model}
	return &application.Judge{Port: client, Tool: settings.Tool, FinalCheck: settings.FinalCheck, Threshold: settings.FinalThreshold, Decisions: decisions}, nil
}

// SettleWith hands Jev to the settlement under the final check's policy:
// the port alone would leave the settlement asking the legacy questions.
func SettleWith(c *application.SessionCeremony, judge *application.Judge) {
	c.Judge, c.Decisions = judge.Port, judge.Decisions
}
