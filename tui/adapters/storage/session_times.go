package storage

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/underpass-ai/AXLR/tui/domain"
)

const sessionTimesVersion = 1
const maxSessionTimesBytes = 1 << 20

// sessionTimes is a session's message times, kept beside its snapshot in
// <id>.times so the snapshot format, which rejects unknown fields, does not
// change and older consoles keep reading every session.
type sessionTimes struct {
	Version int      `json:"version"`
	Times   []string `json:"times"`
}

func (s *SessionStore) timesPath(id domain.SessionID) string {
	return filepath.Join(s.dir, string(id)+".times")
}

// readTimes returns nil when the file is missing or unreadable: times are
// display metadata and never block loading a session.
func (s *SessionStore) readTimes(id domain.SessionID) []time.Time {
	file, err := openNoFollow(s.timesPath(id), os.O_RDONLY, 0)
	if err != nil {
		return nil
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxSessionTimesBytes+1))
	if err != nil || len(data) > maxSessionTimesBytes {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var record sessionTimes
	if decoder.Decode(&record) != nil || record.Version != sessionTimesVersion {
		return nil
	}
	times := make([]time.Time, len(record.Times))
	for i, value := range record.Times {
		if value == "" {
			continue
		}
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return nil
		}
		times[i] = parsed.UTC()
	}
	return times
}

func (s *SessionStore) writeTimes(id domain.SessionID, times []time.Time) error {
	if len(times) == 0 {
		if err := os.Remove(s.timesPath(id)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	record := sessionTimes{Version: sessionTimesVersion, Times: make([]string, len(times))}
	for i, t := range times {
		if !t.IsZero() {
			record.Times[i] = t.UTC().Format(time.RFC3339)
		}
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(s.dir, ".times-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return s.replace(file.Name(), s.timesPath(id))
}
