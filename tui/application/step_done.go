package application

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type stepDone struct {
	CheckCommand *struct {
		Program string   `json:"program"`
		Args    []string `json:"args"`
	} `json:"check_command"`
	Reproducible *bool                `json:"reproducible"`
	Feasible     *bool                `json:"feasible"`
	Expected     string               `json:"expected"`
	Observed     string               `json:"observed"`
	RootCause    string               `json:"root_cause"`
	Evidence     string               `json:"evidence"`
	ProposedFix  string               `json:"proposed_fix"`
	Criteria     string               `json:"criteria"`
	Scope        string               `json:"scope"`
	Summary      string               `json:"summary"`
	Report       string               `json:"report"`
	SummaryEN    string               `json:"summary_en"`
	ConnectTo    []memoryLinkArgument `json:"connect_to"`
	incidentDone
	planDone
	taskDone
}

func decodeStepDone(arguments root.JSONValue) (stepDone, error) {
	var done stepDone
	decoder := json.NewDecoder(bytes.NewReader(arguments.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&done); err != nil {
		return stepDone{}, fmt.Errorf("axlr_step_done arguments: %w; send only the fields the current step asks for", err)
	}
	return done, nil
}

func (d stepDone) command() (domain.CheckCommand, bool) {
	if d.CheckCommand == nil || strings.TrimSpace(d.CheckCommand.Program) == "" {
		return domain.CheckCommand{}, false
	}
	return domain.CheckCommand{Program: d.CheckCommand.Program, Args: append([]string(nil), d.CheckCommand.Args...)}, true
}
