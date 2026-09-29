package diagnostics

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/underpass-ai/AXLR/tui/application"
)

// FileLogger writes one complete JSON object per line. It owns the file.
type FileLogger struct {
	mu      sync.Mutex
	file    *os.File
	failure error
}

// Open creates or appends to a regular file with owner-only permissions.
// Symlinks are rejected so a trace path cannot redirect logs elsewhere.
func Open(path string) (*FileLogger, error) {
	if path == "" {
		return nil, errors.New("diagnostic log path is empty")
	}
	fd, err := syscall.Open(path, syscall.O_WRONLY|syscall.O_APPEND|syscall.O_CREAT|syscall.O_CLOEXEC|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open diagnostic log: %w", err)
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("stat diagnostic log: %w", err)
	}
	if !info.Mode().IsRegular() {
		file.Close()
		return nil, errors.New("diagnostic log is not a regular file")
	}
	if err := file.Chmod(0o600); err != nil {
		file.Close()
		return nil, fmt.Errorf("secure diagnostic log: %w", err)
	}
	return &FileLogger{file: file}, nil
}

// Record validates all string labels before serializing them. Callers may
// choose to ignore a write failure without interrupting the TUI.
func (logger *FileLogger) Record(event application.DiagnosticEvent) error {
	if !event.Stage.Valid() || !event.ErrorClass.Valid() {
		return errors.New("invalid diagnostic event label")
	}
	if event.Chunks < 0 || event.Bytes < 0 || event.ElapsedMilliseconds < 0 || event.Width < 0 || event.Height < 0 {
		return errors.New("negative diagnostic measurement")
	}
	record := struct {
		Timestamp time.Time `json:"timestamp"`
		application.DiagnosticEvent
	}{Timestamp: time.Now().UTC(), DiagnosticEvent: event}
	line, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode diagnostic event: %w", err)
	}
	line = append(line, '\n')
	logger.mu.Lock()
	defer logger.mu.Unlock()
	if logger.file == nil {
		return errors.New("diagnostic log is closed")
	}
	if logger.failure != nil {
		return logger.failure
	}
	_, err = logger.file.Write(line)
	if err != nil {
		logger.failure = fmt.Errorf("write diagnostic log: %w", err)
		return logger.failure
	}
	return nil
}

func (logger *FileLogger) Close() error {
	logger.mu.Lock()
	defer logger.mu.Unlock()
	if logger.file == nil {
		return nil
	}
	file := logger.file
	logger.file = nil
	syncErr := file.Sync()
	closeErr := file.Close()
	return errors.Join(logger.failure, syncErr, closeErr)
}
