package main

import (
	"github.com/underpass-ai/AXLR/tui/adapters/storage"
	"github.com/underpass-ai/AXLR/tui/adapters/terminal"
	"github.com/underpass-ai/AXLR/tui/adapters/typesafe"
	"github.com/underpass-ai/AXLR/tui/application"
)

// jevInfo is what /axlr-info says of Jev: off without a judge.
func jevInfo(judge *application.Judge, settings *storage.JevSettings) terminal.JevInfo {
	if judge == nil || settings == nil {
		return terminal.JevInfo{}
	}
	info := terminal.JevInfo{Model: settings.Model, Tool: judge.Tool, FinalCheck: judge.FinalCheck, Threshold: judge.Threshold, Policy: string(judge.Decisions.Policy), Supersede: judge.Decisions.Supersede}
	if info.Model == "" {
		info.Model = typesafe.DefaultModel
	}
	if info.Threshold == 0 {
		info.Threshold = application.DefaultJudgeThreshold
	}
	return info
}
