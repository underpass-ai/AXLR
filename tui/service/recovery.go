package service

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/underpass-ai/AXLR/tui/domain"
)

// recoverSessions persists the restart transition once. A later restart also
// repairs a journal event if the process stopped after saving the snapshot.
func recoverSessions(sessions *sessionStore, events *eventStore) error {
	ctx := context.Background()
	summaries, err := sessions.List(ctx)
	if err != nil {
		return err
	}
	for _, summary := range summaries {
		session, err := sessions.Load(ctx, summary.ID)
		if err != nil {
			return err
		}
		state := session.Export()
		if state.Status == domain.StatusStreaming || state.Status == domain.StatusApproval {
			if err := session.PauseTurn(); err != nil {
				return err
			}
			if err := sessions.Save(ctx, session); err != nil {
				return err
			}
		}
		if state.OperationID == "" || session.Status() != domain.StatusInterrupted {
			continue
		}
		history, _, err := events.Read(string(state.ID), 0)
		if err != nil {
			return err
		}
		terminal := false
		for i := len(history) - 1; i >= 0; i-- {
			if history[i].OperationID != state.OperationID {
				continue
			}
			switch history[i].Type {
			case "turn.completed", "turn.interrupted", "approval.required", "operation.failed":
				terminal = true
			}
			break
		}
		if !terminal {
			if _, err := events.Append(string(state.ID), state.OperationID, "turn.interrupted", map[string]any{"status": "interrupted"}); err != nil {
				return err
			}
		}
	}
	return nil
}

// recoverClaims makes a persisted key with no corresponding resource visible
// as an interrupted intent. Such a request was never handed to a worker.
func recoverClaims(keys *idempotencyStore, calls *callStore, events *eventStore) error {
	records, err := keys.Records()
	if err != nil {
		return err
	}
	for _, record := range records {
		if _, err := domain.NewSessionID(record.Resource); err != nil {
			return err
		}
		if record.Path == "/v1/tool-calls" {
			if _, err := calls.Load(record.Resource); errors.Is(err, os.ErrNotExist) {
				if err := calls.Save(toolCall{ID: record.Resource, Owner: record.Principal, Tool: "unavailable", Status: "cancelled", Revision: 1, Result: &domain.ToolOutcome{Content: "call intent was interrupted before persistence", IsError: true}}); err != nil {
					return err
				}
			} else if err != nil {
				return err
			}
			continue
		}
		parts := strings.Split(record.Path, "/")
		if len(parts) < 5 || parts[0] != "" || parts[1] != "v1" || parts[2] != "sessions" || !((len(parts) == 5 && parts[4] == "turns") || (len(parts) == 6 && parts[4] == "approvals")) {
			continue
		}
		if _, err := domain.NewSessionID(parts[3]); err != nil {
			return err
		}
		history, _, err := events.Read(parts[3], 0)
		if err != nil {
			return err
		}
		terminal := false
		for _, event := range history {
			if event.OperationID != record.Resource {
				continue
			}
			switch event.Type {
			case "turn.completed", "turn.interrupted", "approval.required", "operation.failed":
				terminal = true
			}
		}
		if !terminal {
			if _, err := events.Append(parts[3], record.Resource, "operation.failed", map[string]any{"code": "interrupted_before_start"}); err != nil {
				return err
			}
		}
	}
	return nil
}
