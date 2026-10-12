package terminal

import (
	"fmt"
	"strings"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// consoleLineRow draws a line the console added to the session for the
// person, such as a finished plan's outcome. On 10 October 2026 a plan
// synced green and its planning session still ended with the planner's
// "It's waiting for your approval on the plan card".
func consoleLineRow(line domain.ConsoleLine, theme Theme) transcriptRow {
	row := transcriptRow{Kind: transcriptRowPlain, Indent: true, At: line.At}
	if plan := line.Plan; plan != nil {
		row.Label, row.LabelTone = theme.Icon("done")+" ", toneGood
		if plan.Status != domain.PlanDone {
			row.Label, row.LabelTone = theme.Icon("attention")+" ", toneWarning
		}
		cost := theme.T("transcript.planCostUnknown")
		if plan.CostKnown {
			cost = theme.Tf("transcript.planCost", FormatUSD(plan.CostUSD))
		}
		row.Text = theme.Tf("transcript.planOutcome", plan.PlanID, theme.T("plans.status."+string(plan.Status)), plan.Done, plan.Tasks, plan.Blocked, plan.Skipped, syncVerdicts(plan.Syncs), changedFiles(plan.Changed), cost)
	}
	if restore := line.Restore; restore != nil {
		// Ctrl+D shows the restore's diff beside the steps.
		row.Label, row.LabelTone = theme.Icon("console")+" ", toneAccent
		row.Text = restoreLineText(*restore, theme)
	}
	if failure := line.Failure; failure != nil {
		// A failed turn stays in the transcript after the footer moves on:
		// on 12 October 2026 an invalid model id left a prompt with no reply
		// and no reason.
		row.Label, row.LabelTone = theme.Icon("failed")+" ", toneError
		row.Text = failure.Message
		if hint := theme.providerHint(failure.Hint); hint != "" {
			row.Text += " · " + hint
		}
	}
	return row
}

// syncVerdicts reads "green" for one wave and "w1 green, w2 red" for more.
func syncVerdicts(syncs []domain.SyncVerdict) string {
	switch len(syncs) {
	case 0:
		return "—"
	case 1:
		return syncs[0].Verdict
	}
	parts := make([]string, len(syncs))
	for i, sync := range syncs {
		parts[i] = fmt.Sprintf("w%d %s", sync.Wave, sync.Verdict)
	}
	return strings.Join(parts, ", ")
}

// changedFiles names the first files a plan changed and counts the rest.
func changedFiles(paths []string) string {
	const shown = 3
	if len(paths) == 0 {
		return "—"
	}
	if len(paths) <= shown {
		return strings.Join(paths, ", ")
	}
	return fmt.Sprintf("%s +%d", strings.Join(paths[:shown], ", "), len(paths)-shown)
}
