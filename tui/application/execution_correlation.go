package application

import "github.com/underpass-ai/AXLR/tui/domain"

// Correlation labels: the execution's ids as the session's MADE record and
// its KMP entries carry them (docs/runtime-contract.md, "Correlation").
const (
	correlationBotLabel       = "bot_id"
	correlationTaskLabel      = "task_id"
	correlationExecutionLabel = "execution_id"
	correlationTraceLabel     = "trace_id"
)

// correlationLabels are the session's correlation ids by label, the given
// ones only; nil without a correlation.
func correlationLabels(s domain.Session) map[string]string {
	correlation, has := s.Correlation()
	if !has {
		return nil
	}
	labels := map[string]string{}
	for label, id := range map[string]string{correlationBotLabel: correlation.BotID, correlationTaskLabel: correlation.TaskID, correlationExecutionLabel: correlation.ExecutionID, correlationTraceLabel: correlation.TraceID} {
		if id != "" {
			labels[label] = id
		}
	}
	return labels
}

// labelCorrelation adds the session's correlation ids to a journal
// entry's labels.
func labelCorrelation(labels map[string][]string, s domain.Session) map[string][]string {
	for label, id := range correlationLabels(s) {
		labels[label] = []string{id}
	}
	return labels
}
