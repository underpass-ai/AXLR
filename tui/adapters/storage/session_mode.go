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

const sessionModeVersion = 1
const maxSessionModeBytes = 4 << 10

// sessionMode is kept in <id>.mode for the same reason as <id>.times: the
// snapshot rejects unknown fields, and older consoles must keep loading.
type sessionMode struct {
	Version int    `json:"version"`
	Mode    string `json:"mode"`
}

func (s *SessionStore) modePath(id domain.SessionID) string {
	return filepath.Join(s.dir, string(id)+".mode")
}

// readMode returns the zero WorkMode when the file is missing or unreadable:
// a mode is a preference and never blocks loading a session, and the zero
// value reads as ModeNormal through Session.Mode() exactly like a session
// that never had its mode sidecar written.
func (s *SessionStore) readMode(id domain.SessionID) domain.WorkMode {
	file, err := openNoFollow(s.modePath(id), os.O_RDONLY, 0)
	if err != nil {
		return ""
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxSessionModeBytes+1))
	if err != nil || len(data) > maxSessionModeBytes {
		return ""
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var record sessionMode
	if decoder.Decode(&record) != nil || record.Version != sessionModeVersion {
		return ""
	}
	mode, err := domain.ParseWorkMode(record.Mode)
	if err != nil {
		return ""
	}
	return mode
}

func (s *SessionStore) writeMode(id domain.SessionID, mode domain.WorkMode) error {
	if mode == "" || mode == domain.ModeNormal {
		if err := os.Remove(s.modePath(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	data, err := json.Marshal(sessionMode{Version: sessionModeVersion, Mode: string(mode)})
	if err != nil {
		return err
	}
	return s.writeSidecar(".mode-*", s.modePath(id), data)
}
