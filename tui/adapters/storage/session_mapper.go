package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
	"github.com/underpass-ai/AXLR/tui/dto"
)

const snapshotVersion = 2

func encodeCall(c root.ToolCall) dto.ToolCall {
	return dto.ToolCall{ID: string(c.ID), Name: string(c.Name), Arguments: c.Arguments.Bytes()}
}
func decodeCall(c dto.ToolCall) (root.ToolCall, error) {
	args, e := root.NewJSONObject(compactJSON(c.Arguments))
	return root.ToolCall{ID: root.ToolCallID(c.ID), Name: root.ToolName(c.Name), Arguments: args}, e
}

// compactJSON drops the layout of a raw value: an indented session bundle
// indents a call's arguments differently in the transcript and in the
// activity, and the restore compares them byte for byte (on 14 October 2026
// every bundle with a tool call failed to import). The store writes
// compact JSON already.
func compactJSON(raw []byte) []byte {
	var out bytes.Buffer
	if json.Compact(&out, raw) != nil {
		return raw
	}
	return out.Bytes()
}

func snapshot(s domain.Session) (dto.SessionSnapshot, error) {
	state := s.Export()
	if _, e := domain.RestoreSession(state); e != nil {
		return dto.SessionSnapshot{}, e
	}
	d := dto.SessionSnapshot{Version: snapshotVersion, ID: string(state.ID), Owner: state.Owner, Revision: state.Revision, OperationID: state.OperationID, Workspace: string(state.Workspace), Model: string(state.Model), Status: string(state.Status), Draft: string(state.Draft), TurnCallCount: state.TurnCallCount}
	for _, m := range state.Messages {
		record := dto.Message{Role: string(m.Role), Content: string(m.Content), ToolCallID: string(m.ToolCallID)}
		for _, c := range m.ToolCalls {
			record.ToolCalls = append(record.ToolCalls, encodeCall(c))
		}
		d.Messages = append(d.Messages, record)
	}
	for _, archived := range state.ArchivedDrafts {
		d.ArchivedDrafts = append(d.ArchivedDrafts, dto.ArchivedDraft{AfterMessage: archived.AfterMessage, Content: string(archived.Content)})
	}
	for _, t := range state.ToolSnapshot {
		tool := dto.AvailableTool{Name: string(t.Definition.Name), Description: string(t.Definition.Description), Parameters: t.Definition.Parameters.Bytes(), Kind: t.Identity.Kind, LocalOperation: t.Identity.LocalOperation, PluginID: string(t.Identity.Plugin.PluginID), PluginToolName: string(t.Identity.Plugin.ToolName)}
		if t.Surface != (domain.ToolSurface{}) {
			tool.Surface = &dto.ToolSurface{Role: string(t.Surface.Role), Hidden: t.Surface.Hidden, ContextArgument: t.Surface.ContextArgument, MemoryWrite: t.Surface.MemoryWrite}
		}
		d.ToolSnapshot = append(d.ToolSnapshot, tool)
	}
	for _, p := range state.Activity {
		record := dto.ToolActivity{Call: encodeCall(p.Call), Decision: string(p.Decision)}
		if p.Outcome != nil {
			record.Outcome = &dto.ToolOutcome{Content: string(p.Outcome.Content), IsError: p.Outcome.IsError, Uncertain: p.Outcome.Uncertain}
			if c := p.Outcome.Change; c != nil {
				record.Outcome.Change = &dto.FileChange{Path: string(c.Path), Before: string(c.Before), After: string(c.After), Created: c.Created, Unavailable: c.Unavailable}
			}
		}
		d.Activity = append(d.Activity, record)
	}
	return d, nil
}
func restore(d dto.SessionSnapshot) (domain.Session, error) {
	return restoreSnapshot(d, false, nil, "", nil, nil, consoleNotes{})
}

func restoreSnapshot(d dto.SessionSnapshot, preserveActive bool, times []time.Time, mode domain.WorkMode, ceremony *domain.CeremonyRun, instance *domain.SessionInstance, notes consoleNotes) (domain.Session, error) {
	if d.Version != 1 && d.Version != snapshotVersion && d.Version != storedSnapshotVersion {
		return domain.Session{}, errors.New("unsupported session snapshot version")
	}
	if d.ToolSnapshotRef != "" {
		return domain.Session{}, errors.New("the snapshot names a stored tool set instead of carrying its tools")
	}
	s := domain.SessionState{ID: domain.SessionID(d.ID), Owner: d.Owner, Revision: d.Revision, OperationID: d.OperationID, Workspace: domain.Workspace(d.Workspace), Model: root.ModelID(d.Model), Status: domain.SessionStatus(d.Status), Draft: root.Text(d.Draft), TurnCallCount: d.TurnCallCount}
	for _, m := range d.Messages {
		record := root.Message{Role: root.MessageRole(m.Role), Content: root.Text(m.Content), ToolCallID: root.ToolCallID(m.ToolCallID)}
		for _, c := range m.ToolCalls {
			call, e := decodeCall(c)
			if e != nil {
				return domain.Session{}, e
			}
			record.ToolCalls = append(record.ToolCalls, call)
		}
		s.Messages = append(s.Messages, record)
	}
	for _, archived := range d.ArchivedDrafts {
		s.ArchivedDrafts = append(s.ArchivedDrafts, domain.ArchivedDraft{AfterMessage: archived.AfterMessage, Content: root.Text(archived.Content)})
	}
	for _, t := range d.ToolSnapshot {
		params, e := root.NewJSONObject(compactJSON(t.Parameters))
		if e != nil {
			return domain.Session{}, e
		}
		tool := domain.AvailableTool{Definition: root.ToolDefinition{Name: root.ToolName(t.Name), Description: root.Text(t.Description), Parameters: params}, Identity: domain.ToolIdentity{Kind: t.Kind, LocalOperation: t.LocalOperation, Plugin: root.PluginRef{PluginID: root.PluginID(t.PluginID), ToolName: root.PluginToolName(t.PluginToolName)}}}
		if t.Surface != nil {
			tool.Surface = domain.ToolSurface{Role: domain.PluginRole(t.Surface.Role), Hidden: t.Surface.Hidden, ContextArgument: t.Surface.ContextArgument, MemoryWrite: t.Surface.MemoryWrite}
		}
		s.ToolSnapshot = append(s.ToolSnapshot, tool)
	}
	for _, p := range d.Activity {
		call, e := decodeCall(p.Call)
		if e != nil {
			return domain.Session{}, e
		}
		record := domain.PendingTool{Call: call, Decision: domain.ToolDecision(p.Decision)}
		if p.Outcome != nil {
			record.Outcome = &domain.ToolOutcome{Content: root.Text(p.Outcome.Content), IsError: p.Outcome.IsError, Uncertain: p.Outcome.Uncertain}
			if c := p.Outcome.Change; c != nil {
				record.Outcome.Change = &domain.FileChange{Path: root.RelativePath(c.Path), Before: root.Text(c.Before), After: root.Text(c.After), Created: c.Created, Unavailable: c.Unavailable}
			}
		}
		s.Activity = append(s.Activity, record)
	}
	s.MessageTimes = times
	s.Mode = mode
	s.Ceremony = ceremony
	s.Instance = instance
	notes.apply(&s)
	restored, err := restoreState(s, preserveActive)
	if err != nil && len(s.ConsoleLines) > 0 {
		// Console lines that no longer fit the transcript are dropped
		// rather than the session: they are only ever shown.
		s.ConsoleLines = nil
		return restoreState(s, preserveActive)
	}
	return restored, err
}

func restoreState(s domain.SessionState, preserveActive bool) (domain.Session, error) {
	if preserveActive {
		return domain.RestoreSessionActive(s)
	}
	return domain.RestoreSession(s)
}
