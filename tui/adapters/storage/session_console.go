package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/underpass-ai/AXLR/tui/domain"
)

const sessionConsoleVersion = 1

const maxSessionConsoleBytes = 64 << 10

// sessionConsole is what the console keeps for the person beside the
// snapshot, in <id>.console, for the same reason as <id>.mode: the snapshot
// rejects unknown fields, and older consoles must keep loading.
type sessionConsole struct {
	Version int `json:"version"`
	// StoppedCeremony is the ceremony the person stopped since the last
	// prompt; the next prompt tells the model.
	StoppedCeremony string `json:"stopped_ceremony,omitempty"`
}

// consoleNotes is the part of a session's state <id>.console holds.
type consoleNotes struct {
	StoppedCeremony string
}

func (n consoleNotes) empty() bool { return n.StoppedCeremony == "" }

func notesOf(state domain.SessionState) consoleNotes {
	return consoleNotes{StoppedCeremony: state.StoppedCeremony}
}

func (n consoleNotes) apply(state *domain.SessionState) {
	state.StoppedCeremony = n.StoppedCeremony
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
	return consoleNotes{StoppedCeremony: record.StoppedCeremony}
}

func (s *SessionStore) writeConsole(id domain.SessionID, notes consoleNotes) error {
	if notes.empty() {
		if err := os.Remove(s.consolePath(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	data, err := json.Marshal(sessionConsole{Version: sessionConsoleVersion, StoppedCeremony: notes.StoppedCeremony})
	if err != nil {
		return err
	}
	return s.writeSidecar(".console-*", s.consolePath(id), data)
}
