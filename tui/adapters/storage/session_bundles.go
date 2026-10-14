package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// A session bundle (decision I4 of the lifecycle integration design) is
// the session as the store keeps it, the snapshot and its sidecars, with
// the title, the about and where it came from: /session export writes
// one, axlr-tui --import reads it. The driven ceremony is left out, it is
// bound to the origin's MADE; the instance travels for its successor to
// name it as predecessor.
const (
	sessionBundleVersion  = 1
	maxSessionBundleBytes = 64 << 20
)

// SessionBundles encodes and decodes session bundles.
type SessionBundles struct{}

var _ application.SessionBundleCodec = SessionBundles{}

// Encode writes the session and its sidecars as one JSON document.
func (SessionBundles) Encode(session domain.Session, meta application.SessionBundleMeta) ([]byte, error) {
	record, err := snapshot(session)
	if err != nil {
		return nil, err
	}
	state := session.Export()
	bundle := sessionBundle{Version: sessionBundleVersion, ExportedAt: meta.ExportedAt.UTC().Format(time.RFC3339), Host: meta.Host, Title: meta.Title, About: meta.About, Snapshot: record, Mode: string(session.Mode())}
	for _, t := range state.MessageTimes {
		if t.IsZero() {
			bundle.Times = append(bundle.Times, "")
		} else {
			bundle.Times = append(bundle.Times, t.UTC().Format(time.RFC3339))
		}
	}
	if instance := state.Instance; instance != nil {
		bundle.Instance = &sessionInstance{Version: sessionInstanceVersion, Instance: instance.Instance, Turns: instance.Turns, Fence: instance.Fence, TurnIndex: instance.TurnIndex, ClaimRequests: instance.ClaimRequests, ClaimCost: instance.ClaimCost, Closed: instance.Closed, LastRecord: instance.LastRecord, Unavailable: instance.Unavailable, Pending: instance.Pending, MemoryAgent: string(instance.MemoryAgent), MemoryContext: string(instance.MemoryContext), Recalled: instance.Recalled, OperationID: instance.OperationID, ClaimedAt: formatInstant(instance.ClaimedAt), LeaseMS: instance.LeaseMS, ReportedAt: formatInstant(instance.Reported), Reports: instance.Reports, Incarnation: instance.ClaimIncarnation, Recalled2: instance.RecalledMemories, Predecessor: string(instance.Predecessor)}
	}
	if notes := notesOf(state); !notes.empty() {
		bundle.Console = &sessionConsole{Version: sessionConsoleVersion, StoppedCeremony: notes.StoppedCeremony, ChosenCeremony: notes.Chosen.Definition, ChosenVersion: notes.Chosen.Version, Lines: encodeConsoleLines(notes.Lines)}
	}
	// Arguments keep their bytes (<, > and &) as the store keeps them.
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", " ")
	if err := encoder.Encode(bundle); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// Decode reads a bundle back into a session, its instance as the origin
// left it; the import use case makes it this console's.
func (SessionBundles) Decode(data []byte) (domain.Session, application.SessionBundleMeta, error) {
	if len(data) > maxSessionBundleBytes {
		return domain.Session{}, application.SessionBundleMeta{}, fmt.Errorf("the bundle exceeds %d bytes", maxSessionBundleBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	var bundle sessionBundle
	if err := decoder.Decode(&bundle); err != nil {
		return domain.Session{}, application.SessionBundleMeta{}, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return domain.Session{}, application.SessionBundleMeta{}, errors.New("trailing bundle data")
	}
	if bundle.Version != sessionBundleVersion {
		return domain.Session{}, application.SessionBundleMeta{}, fmt.Errorf("unsupported session bundle version %d", bundle.Version)
	}
	meta := application.SessionBundleMeta{Host: bundle.Host, Title: bundle.Title, About: bundle.About}
	if bundle.ExportedAt != "" {
		if at, err := time.Parse(time.RFC3339, bundle.ExportedAt); err == nil {
			meta.ExportedAt = at.UTC()
		}
	}
	var times []time.Time
	for _, value := range bundle.Times {
		var t time.Time
		if value != "" {
			parsed, err := time.Parse(time.RFC3339, value)
			if err != nil {
				return domain.Session{}, meta, fmt.Errorf("message time %q: %w", value, err)
			}
			t = parsed.UTC()
		}
		times = append(times, t)
	}
	var instance *domain.SessionInstance
	if record := bundle.Instance; record != nil {
		decoded := domain.SessionInstance{Instance: record.Instance, Turns: record.Turns, Fence: record.Fence, TurnIndex: record.TurnIndex, ClaimRequests: record.ClaimRequests, ClaimCost: record.ClaimCost, Closed: record.Closed, LastRecord: record.LastRecord, Unavailable: record.Unavailable, Pending: record.Pending, MemoryAgent: domain.MemoryAgentID(record.MemoryAgent), MemoryContext: domain.MemoryContextID(record.MemoryContext), Recalled: record.Recalled, OperationID: record.OperationID, ClaimedAt: parseInstant(record.ClaimedAt), LeaseMS: record.LeaseMS, Reported: parseInstant(record.ReportedAt), Reports: record.Reports, ClaimIncarnation: record.Incarnation, RecalledMemories: record.Recalled2, Predecessor: domain.SessionID(record.Predecessor)}
		if err := decoded.Validate(); err != nil {
			return domain.Session{}, meta, fmt.Errorf("session instance: %w", err)
		}
		instance = &decoded
	}
	notes := consoleNotes{}
	if console := bundle.Console; console != nil {
		notes = consoleNotes{StoppedCeremony: console.StoppedCeremony, Chosen: domain.CeremonyChoice{Definition: console.ChosenCeremony, Version: console.ChosenVersion}, Lines: decodeConsoleLines(console.Lines)}
	}
	session, err := restoreSnapshot(bundle.Snapshot, false, times, domain.WorkMode(bundle.Mode), nil, instance, notes)
	return session, meta, err
}
