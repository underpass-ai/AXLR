package terminal

import (
	"encoding/json"
	"strings"
)

// checkCommandUse is one distinct command an axlr_step_done call asks the
// person to approve, and what in the hand-back names it.
type checkCommandUse struct {
	Command string
	E2E     bool
	Tasks   []string
}

// stepDoneCommands reads the commands an axlr_step_done call would run once
// approved: a brief's or a reproduction's check_command, or a decompose's
// end-to-end check and every task's unit check, each command once in the
// order it first appears. ok is false for a call that names none.
func stepDoneCommands(arguments []byte) ([]checkCommandUse, bool) {
	type command struct {
		Program string   `json:"program"`
		Args    []string `json:"args"`
	}
	var call struct {
		CheckCommand *command `json:"check_command"`
		E2ECheck     *command `json:"e2e_check"`
		Tasks        []struct {
			ID        string   `json:"id"`
			UnitCheck *command `json:"unit_check"`
		} `json:"tasks"`
	}
	if json.Unmarshal(arguments, &call) != nil {
		return nil, false
	}
	var uses []checkCommandUse
	add := func(c *command) *checkCommandUse {
		if c == nil || strings.TrimSpace(c.Program) == "" {
			return nil
		}
		line := formatCommand(c.Program, c.Args)
		for i := range uses {
			if uses[i].Command == line {
				return &uses[i]
			}
		}
		uses = append(uses, checkCommandUse{Command: line})
		return &uses[len(uses)-1]
	}
	add(call.CheckCommand)
	if use := add(call.E2ECheck); use != nil {
		use.E2E = true
	}
	for _, task := range call.Tasks {
		if use := add(task.UnitCheck); use != nil && task.ID != "" {
			use.Tasks = append(use.Tasks, task.ID)
		}
	}
	return uses, len(uses) > 0
}

// checkCommandsCard lays the check-command card out as the commands it
// approves, then the call's JSON below them for whoever scrolls on. On 10
// October 2026 a decompose card showed ~60 lines of goals, citations and
// interfaces, and the commands it approved came last, ~30 ↓ presses away.
func checkCommandsCard(uses []checkCommandUse, arguments string, theme Theme) string {
	lines := []string{theme.T("approval.checkCommandsLead")}
	for _, use := range uses {
		line := "$ " + use.Command
		var by []string
		if use.E2E {
			by = append(by, theme.T("approval.checkByE2E"))
		}
		switch len(use.Tasks) {
		case 0:
		case 1:
			by = append(by, theme.Tf("approval.checkByTask", use.Tasks[0]))
		default:
			by = append(by, theme.Tf("approval.checkByTasks", strings.Join(use.Tasks, ", ")))
		}
		if len(by) > 0 {
			line += "   · " + strings.Join(by, " · ")
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", theme.T("approval.checkArguments"), arguments)
	return strings.Join(lines, "\n")
}
