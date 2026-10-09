package main

import (
	"github.com/underpass-ai/AXLR/tui/adapters/terminal"
	"github.com/underpass-ai/AXLR/tui/application"
)

// The /jobs panel finds the coordinator's jobs side behind
// Dependencies.Repairs; this keeps the console's coordinator serving it, or
// /jobs would say MADE is not connected while it is.
var _ terminal.JobsPanelPort = (*application.SelfRepair)(nil)
