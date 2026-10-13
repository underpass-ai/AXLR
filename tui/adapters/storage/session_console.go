package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/domain"
)

const sessionConsoleVersion = 1

// maxSessionConsoleBytes holds a long session's plan outcomes; a larger file is
// ignored, which loses the lines, not the session.
const maxSessionConsoleBytes = 1 << 20

// sessionConsole is what the console keeps for the person beside the
// snapshot, in <id>.console, for the same reason as <id>.mode: the snapshot
// rejects unknown fields, and older consoles must keep loading.
type sessionConsole struct {
	Version int `json:"version"`
	// StoppedCeremony is the ceremony the person stopped since the last
	// prompt; the next prompt tells the model.
	StoppedCeremony string `json:"stopped_ceremony,omitempty"`
	// ChosenCeremony and ChosenVersion are the definition /ceremony chose.
	ChosenCeremony string `json:"chosen_ceremony,omitempty"`
	ChosenVersion  string `json:"chosen_version,omitempty"`
	// Lines are the console lines shown between messages, such as a
	// finished plan's outcome; files written before them have none.
	Lines []sessionConsoleLine `json:"lines,omitempty"`
}

type sessionConsoleLine struct {
	AfterMessage int                 `json:"after_message"`
	At           time.Time           `json:"at"`
	Plan         *sessionPlanOutcome `json:"plan,omitempty"`
	Restore      *sessionRestore     `json:"restore,omitempty"`
	Failure      *sessionFailure     `json:"failure,omitempty"`
}

// sessionFailure is a turn that failed: the error and the kind of hint
// the console words for the person.
type sessionFailure struct {
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// sessionRestore names the steps a restore went back over; their
// snapshots stay in the session's activity.
type sessionRestore struct {
	Undo  bool                  `json:"undo,omitempty"`
	Files []sessionRestoredFile `json:"files"`
}

type sessionRestoredFile struct {
	Path string `json:"path"`
	From string `json:"from"`
	To   string `json:"to"`
}

type sessionPlanOutcome struct {
	ID        string               `json:"id"`
	Status    string               `json:"status"`
	Tasks     int                  `json:"tasks"`
	Done      int                  `json:"done"`
	Blocked   int                  `json:"blocked"`
	Skipped   int                  `json:"skipped"`
	Syncs     []sessionSyncVerdict `json:"syncs,omitempty"`
	Changed   []string             `json:"changed,omitempty"`
	CostUSD   float64              `json:"cost_usd,omitempty"`
	CostKnown bool                 `json:"cost_known,omitempty"`
}

type sessionSyncVerdict struct {
	Wave    int    `json:"wave"`
	Verdict string `json:"verdict"`
}

// consoleNotes is the part of a session's state <id>.console holds.
type consoleNotes struct {
	StoppedCeremony string
	Chosen          domain.CeremonyChoice
	Lines           []domain.ConsoleLine
}

func (n consoleNotes) empty() bool {
	return n.StoppedCeremony == "" && n.Chosen == (domain.CeremonyChoice{}) && len(n.Lines) == 0
}

func notesOf(state domain.SessionState) consoleNotes {
	return consoleNotes{StoppedCeremony: state.StoppedCeremony, Chosen: state.ChosenCeremony, Lines: state.ConsoleLines}
}

func (n consoleNotes) apply(state *domain.SessionState) {
	state.StoppedCeremony = n.StoppedCeremony
	state.ChosenCeremony = n.Chosen
	state.ConsoleLines = n.Lines
}

func encodeConsoleLines(lines []domain.ConsoleLine) []sessionConsoleLine {
	var out []sessionConsoleLine
	for _, line := range lines {
		record := sessionConsoleLine{AfterMessage: line.AfterMessage, At: line.At}
		if p := line.Plan; p != nil {
			record.Plan = &sessionPlanOutcome{ID: p.PlanID, Status: string(p.Status), Tasks: p.Tasks, Done: p.Done, Blocked: p.Blocked, Skipped: p.Skipped, Changed: p.Changed, CostUSD: p.CostUSD, CostKnown: p.CostKnown}
			for _, sync := range p.Syncs {
				record.Plan.Syncs = append(record.Plan.Syncs, sessionSyncVerdict{Wave: sync.Wave, Verdict: sync.Verdict})
			}
		}
		if r := line.Restore; r != nil {
			record.Restore = &sessionRestore{Undo: r.Undo}
			for _, f := range r.Files {
				record.Restore.Files = append(record.Restore.Files, sessionRestoredFile{Path: string(f.Path), From: string(f.From), To: string(f.To)})
			}
		}
		if f := line.Failure; f != nil {
			record.Failure = &sessionFailure{Message: f.Message, Hint: f.Hint}
		}
		out = append(out, record)
	}
	return out
}

func decodeConsoleLines(records []sessionConsoleLine) []domain.ConsoleLine {
	var out []domain.ConsoleLine
	for _, record := range records {
		line := domain.ConsoleLine{AfterMessage: record.AfterMessage, At: record.At}
		if p := record.Plan; p != nil {
			line.Plan = &domain.PlanOutcome{PlanID: p.ID, Status: domain.PlanStatus(p.Status), Tasks: p.Tasks, Done: p.Done, Blocked: p.Blocked, Skipped: p.Skipped, Changed: p.Changed, CostUSD: p.CostUSD, CostKnown: p.CostKnown}
			for _, sync := range p.Syncs {
				line.Plan.Syncs = append(line.Plan.Syncs, domain.SyncVerdict{Wave: sync.Wave, Verdict: sync.Verdict})
			}
		}
		if r := record.Restore; r != nil {
			line.Restore = &domain.FileRestore{Undo: r.Undo}
			for _, f := range r.Files {
				line.Restore.Files = append(line.Restore.Files, domain.RestoredFile{Path: root.RelativePath(f.Path), From: root.ToolCallID(f.From), To: root.ToolCallID(f.To)})
			}
		}
		if f := record.Failure; f != nil {
			line.Failure = &domain.TurnFailure{Message: f.Message, Hint: f.Hint}
		}
		out = append(out, line)
	}
	return out
}

func (s *SessionStore) consolePath(id domain.SessionID) string {
	return filepath.Join(s.dir, string(id)+".console")
}

// readConsole returns no notes when the file is missing or unreadable: a
// lost note only loses what the console would have told the person or the
// model, never the session. Lenient like <id>.ceremony, so a console that
// does not know a newer field still reads the others.
func (s *SessionStore) readConsole(id domain.SessionID) consoleNotes {
	file, err := openNoFollow(s.consolePath(id), os.O_RDONLY, 0)
	if err != nil {
		return consoleNotes{}
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxSessionConsoleBytes+1))
	if err != nil || len(data) > maxSessionConsoleBytes {
		return consoleNotes{}
	}
	var record sessionConsole
	if json.NewDecoder(bytes.NewReader(data)).Decode(&record) != nil || record.Version != sessionConsoleVersion {
		return consoleNotes{}
	}
	return consoleNotes{StoppedCeremony: record.StoppedCeremony, Chosen: domain.CeremonyChoice{Definition: record.ChosenCeremony, Version: record.ChosenVersion}, Lines: decodeConsoleLines(record.Lines)}
}

func (s *SessionStore) writeConsole(id domain.SessionID, notes consoleNotes) error {
	if notes.empty() {
		if err := os.Remove(s.consolePath(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	data, err := json.Marshal(sessionConsole{Version: sessionConsoleVersion, StoppedCeremony: notes.StoppedCeremony, ChosenCeremony: notes.Chosen.Definition, ChosenVersion: notes.Chosen.Version, Lines: encodeConsoleLines(notes.Lines)})
	if err != nil {
		return err
	}
	return s.writeSidecar(".console-*", s.consolePath(id), data)
}
