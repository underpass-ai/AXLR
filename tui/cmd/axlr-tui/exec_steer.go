package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"sync"

	root "github.com/underpass-ai/AXLR/domain"
)

// maxSteerMessageBytes caps one message of --steer-file; a longer line is
// skipped.
const maxSteerMessageBytes = 64 << 10

// execSteerFile is --steer-file: what a person writes to a run while it
// works, one message per line, as plain text or {"text": "..."}. The run
// takes what is new between model steps, as the console's queued prompt;
// a message written after the last step gets one more turn. A line counts
// once it ends in a newline, so a writer may append it in pieces. The file
// need not exist when the run starts.
type execSteerFile struct {
	mu     sync.Mutex
	path   string
	offset int64
	queued []root.Text
	// taken tells the run's events that a message went to the model.
	taken func(root.Text)
}

func newExecSteerFile(path string) *execSteerFile {
	return &execSteerFile{path: path}
}

// Take returns every message not taken yet, joined as the console joins
// queued prompts.
func (s *execSteerFile) Take() (root.Text, bool) {
	if s == nil {
		return "", false
	}
	s.mu.Lock()
	s.readNew()
	if len(s.queued) == 0 {
		s.mu.Unlock()
		return "", false
	}
	parts := make([]string, 0, len(s.queued))
	for _, text := range s.queued {
		parts = append(parts, string(text))
	}
	s.queued = nil
	taken := s.taken
	s.mu.Unlock()
	text := root.Text(strings.Join(parts, "\n\n"))
	if taken != nil {
		taken(text)
	}
	return text, true
}

// Restore puts back a message the turn could not keep, ahead of the rest.
func (s *execSteerFile) Restore(text root.Text) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queued = append([]root.Text{text}, s.queued...)
}

// readNew reads the complete lines written since the last read.
func (s *execSteerFile) readNew() {
	file, err := os.Open(s.path)
	if err != nil {
		return
	}
	defer file.Close()
	if _, err := file.Seek(s.offset, io.SeekStart); err != nil {
		return
	}
	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadBytes('\n')
		if errors.Is(err, io.EOF) || err != nil {
			// A line without its newline yet is read again next time.
			return
		}
		s.offset += int64(len(line))
		if text, ok := steerMessage(line); ok {
			s.queued = append(s.queued, text)
		}
	}
}

// steerMessage reads one line: {"text": "..."}, or the line as it is.
func steerMessage(line []byte) (root.Text, bool) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 || len(line) > maxSteerMessageBytes {
		return "", false
	}
	raw := string(line)
	if line[0] == '{' {
		var message struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(line, &message) == nil {
			raw = message.Text
		}
	}
	text, err := root.NewText(strings.TrimSpace(raw))
	if err != nil || text == "" {
		return "", false
	}
	return text, true
}
