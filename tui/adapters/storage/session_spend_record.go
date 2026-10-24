package storage

import "github.com/underpass-ai/AXLR/tui/domain"

type sessionSpend struct {
	Requests  int     `json:"requests"`
	ToolCalls int     `json:"tool_calls"`
	USD       float64 `json:"usd"`
}

func spendRecord(spend domain.ExecutionSpend) sessionSpend {
	return sessionSpend{Requests: spend.Requests, ToolCalls: spend.ToolCalls, USD: spend.USD}
}

func (s sessionSpend) domain() domain.ExecutionSpend {
	return domain.ExecutionSpend{Requests: s.Requests, ToolCalls: s.ToolCalls, USD: s.USD}
}
