package application

import "github.com/underpass-ai/AXLR/tui/domain"

type ceremonySpec struct {
	definition, version, first, briefInput string
}

var ceremonySpecs = map[domain.WorkMode]ceremonySpec{
	domain.ModeDebug:    {definition: "axlr_debug", version: "2.0", first: "reproduce", briefInput: "failure_brief"},
	domain.ModeDelivery: {definition: "axlr_delivery", version: "2.0", first: "brief", briefInput: "task_brief"},
	domain.ModeIncident: {definition: "axlr_incident", version: "1.0", first: "triage", briefInput: "incident_brief"},
	domain.ModeRepair:   {definition: "axlr_repair", version: "1.0", first: "reproduce", briefInput: "failure_brief"},
	domain.ModeImprove:  {definition: "axlr_improve", version: "1.0", first: "brief", briefInput: "improvement_brief"},
	domain.ModePlan:     {definition: "axlr_plan", version: "1.0", first: "decompose", briefInput: "brief"},
}
