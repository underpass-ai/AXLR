package contract

import (
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// The budget and the correlation are read strictly, bare or wrapped, and
// written back as given.
func TestParseExecutionBudgetAndCorrelation(t *testing.T) {
	for _, raw := range []string{`{"seconds":900,"requests":20,"tool_calls":60,"usd":0.5}`, `{"budget":{"seconds":900,"requests":20,"tool_calls":60,"usd":0.5}}`} {
		budget, err := ParseExecutionBudget([]byte(raw))
		if err != nil || budget.Seconds() != 900 || budget.Requests() != 20 || budget.ToolCalls() != 60 || budget.USD() != 0.5 {
			t.Fatalf("%s: %+v %v", raw, budget, err)
		}
		wire := ExecutionBudget(budget)
		if wire.Seconds != 900 || wire.Requests != 20 || wire.ToolCalls != 60 || wire.USD != 0.5 {
			t.Fatalf("%+v", wire)
		}
	}
	if budget, err := ParseExecutionBudget([]byte(`{}`)); err != nil || !budget.IsZero() {
		t.Fatalf("%+v %v", budget, err)
	}
	for name, raw := range map[string]string{
		"unknown field": `{"seconds":900,"minutes":1}`,
		"negative":      `{"requests":-1}`,
		"not an object": `900`,
		"trailing":      `{"seconds":1} {}`,
		"text":          `{"seconds":"900"}`,
		"too large":     `{"seconds":1,"x":"` + strings.Repeat("a", MaxExecutionObjectBytes) + `"}`,
	} {
		if _, err := ParseExecutionBudget([]byte(raw)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	for _, raw := range []string{`{"bot_id":"server-incident-investigator","task_id":"task-789","execution_id":"exec-1","trace_id":"trace-abc"}`, `{"correlation":{"bot_id":"server-incident-investigator","task_id":"task-789","execution_id":"exec-1","trace_id":"trace-abc"}}`} {
		correlation, err := ParseExecutionCorrelation([]byte(raw))
		if err != nil || correlation != (domain.ExecutionCorrelation{BotID: "server-incident-investigator", TaskID: "task-789", ExecutionID: "exec-1", TraceID: "trace-abc"}) {
			t.Fatalf("%s: %+v %v", raw, correlation, err)
		}
		if wire := ExecutionCorrelation(correlation); wire.BotID != "server-incident-investigator" || wire.TraceID != "trace-abc" {
			t.Fatalf("%+v", wire)
		}
	}
	for name, raw := range map[string]string{
		"unknown field": `{"bot_id":"b","span_id":"s"}`,
		"unsafe id":     `{"bot_id":"bot one"}`,
		"not an object": `"bot"`,
		"number":        `{"bot_id":1}`,
	} {
		if _, err := ParseExecutionCorrelation([]byte(raw)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}
