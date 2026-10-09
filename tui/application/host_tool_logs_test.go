package application

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

type logMemory struct {
	queries []AppLogQuery
	entries []string
}

func (l *logMemory) TailLog(_ context.Context, query AppLogQuery) (AppLogPage, error) {
	l.queries = append(l.queries, query)
	return AppLogPage{Path: "/state/axlr/logs/axlr.log", Console: 42, Entries: append([]string(nil), l.entries...)}, nil
}

func TestAxlrLogsIsOfferedWhenTheConsoleKeepsALog(t *testing.T) {
	for _, offered := range []bool{true, false} {
		s := turnSession(t)
		if err := s.BeginTurn("why did kmp fail", turnTools()); err != nil {
			t.Fatal(err)
		}
		var request root.CompletionRequest
		u := ContinueTurnUseCase{Store: &memoryStore{}, Models: streamFunc(func(_ context.Context, req root.CompletionRequest, _ func(root.Text) error) (root.CompletionResult, error) {
			request = req
			return assistant("ok"), nil
		})}
		if offered {
			u.Logs = &logMemory{}
		}
		if err := u.Execute(context.Background(), &s, ignoreEvent); err != nil {
			t.Fatal(err)
		}
		if requestHasTool(request, HostLogsName) != offered {
			t.Fatalf("offered=%v, request has axlr_logs=%v", offered, requestHasTool(request, HostLogsName))
		}
	}
}

func TestAxlrLogsReadsWithoutApprovalAndValidatesItsArguments(t *testing.T) {
	identity, err := domain.NewHostToolIdentity(domain.HostOperationLogs)
	if err != nil {
		t.Fatal(err)
	}
	if !automaticallyApproves(nil, identity) {
		t.Fatal("axlr_logs is read-only and needs no approval")
	}
	logs := &logMemory{entries: []string{"2026-10-09T15:04:05.000Z WARN  [42] plugin kmp: connection failed: closed"}}
	u := HostToolUseCase{Logs: logs}
	s := turnSession(t)
	out, err := u.Execute(context.Background(), s, identity, mustJSON(`{"level":"WARN","contains":"kmp","lines":5}`))
	if err != nil || out.IsError {
		t.Fatalf("%+v %v", out, err)
	}
	var page AppLogPage
	if err := json.Unmarshal([]byte(out.Content), &page); err != nil || page.Console != 42 || len(page.Entries) != 1 || !strings.Contains(page.Entries[0], "plugin kmp") {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	if got := logs.queries[0]; got != (AppLogQuery{Lines: 5, Level: "WARN", Contains: "kmp"}) {
		t.Fatalf("query = %+v", got)
	}
	if _, err := u.Execute(context.Background(), s, identity, mustJSON(`{}`)); err != nil || logs.queries[1] != (AppLogQuery{Lines: 50, Level: "INFO"}) {
		t.Fatalf("defaults = %+v %v", logs.queries[1], err)
	}
	for _, bad := range []string{`{"lines":0}`, `{"lines":201}`, `{"level":"DEBUG"}`, `{"path":"/etc/passwd"}`} {
		out, err := u.Execute(context.Background(), s, identity, mustJSON(bad))
		if err != nil || !out.IsError {
			t.Fatalf("%s accepted: %+v %v", bad, out, err)
		}
	}
	out, err = (HostToolUseCase{}).Execute(context.Background(), s, identity, mustJSON(`{}`))
	if err != nil || !out.IsError || !strings.Contains(string(out.Content), "no app log") {
		t.Fatalf("without a log: %+v %v", out, err)
	}
}

// A page keeps the newest entries that fit the host result bound and counts
// the rest as omitted.
func TestAxlrLogsKeepsTheNewestEntriesThatFit(t *testing.T) {
	logs := &logMemory{}
	for i := 0; i < 200; i++ {
		logs.entries = append(logs.entries, strings.Repeat("x", 4000))
	}
	logs.entries[199] = "newest"
	result, err := hostLogs(context.Background(), logs, mustJSON(`{"lines":200}`))
	if err != nil {
		t.Fatal(err)
	}
	page := result.(AppLogPage)
	encoded, _ := json.Marshal(page)
	if len(encoded) > MaxHostResultBytes || page.Entries[len(page.Entries)-1] != "newest" || page.Omitted+len(page.Entries) != 200 || page.Omitted == 0 {
		t.Fatalf("bytes=%d entries=%d omitted=%d", len(encoded), len(page.Entries), page.Omitted)
	}
}
