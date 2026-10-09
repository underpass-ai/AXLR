package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/underpass-ai/AXLR/adapters/local"
	"github.com/underpass-ai/AXLR/application"
	"github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/dto"
)

// Executor is a serial library entrypoint for the trusted-local profile.
type Executor struct {
	files     *local.FileAdapter
	processes *local.ProcessAdapter
	config    Config
	mu        sync.Mutex
}

func New(c Config) (*Executor, error) {
	if c.Root == "" {
		return nil, errors.New("workspace root is required")
	}
	if c.MaxReadBytes == 0 {
		c.MaxReadBytes = hardFileBytes
	}
	if c.MaxFileBytes == 0 {
		c.MaxFileBytes = hardFileBytes
	}
	if c.MaxOutputBytes == 0 {
		c.MaxOutputBytes = hardFileBytes
	}
	if c.MaxTimeout == 0 {
		c.MaxTimeout = HardTimeout
	}
	if c.MaxReadBytes < 1 || c.MaxReadBytes > hardFileBytes || c.MaxFileBytes < 1 || c.MaxFileBytes > hardFileBytes || c.MaxOutputBytes < 1 || c.MaxOutputBytes > hardFileBytes || c.MaxTimeout < time.Millisecond || c.MaxTimeout > HardTimeout {
		return nil, errors.New("profile limits exceed trusted-local bounds")
	}
	for _, v := range c.Env {
		k, _, ok := strings.Cut(v, "=")
		if !ok || k == "" || strings.ContainsAny(k, "\x00=") || strings.ContainsRune(v, '\x00') {
			return nil, errors.New("invalid child environment")
		}
	}
	path, err := filepath.EvalSymlinks(c.Root)
	if err != nil {
		return nil, err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("workspace root is not a directory")
	}
	files, err := local.NewFileAdapter(path)
	if err != nil {
		return nil, err
	}
	c.Root = path
	c.Env = append([]string{}, c.Env...)
	return &Executor{files: files, processes: &local.ProcessAdapter{Root: path, Env: c.Env}, config: c}, nil
}
func (e *Executor) Close() error { e.mu.Lock(); defer e.mu.Unlock(); return e.files.Close() }

// Execute never interprets request_id as an idempotency key.
func (e *Executor) Execute(ctx context.Context, req dto.Request) dto.Response {
	e.mu.Lock()
	defer e.mu.Unlock()
	start := time.Now()
	r := dto.Response{ProtocolVersion: ProtocolVersion, RequestID: req.RequestID, Tool: req.Tool, StartedAt: start.UTC(), Status: "completed"}
	mapper := RequestMapper{MaxReadBytes: e.config.MaxReadBytes, MaxFileBytes: e.config.MaxFileBytes, MaxOutputBytes: e.config.MaxOutputBytes, MaxTimeout: e.config.MaxTimeout}
	command, err := mapper.Map(req)
	var result any
	listingBytes := 0
	if err == nil {
		if ctx.Err() != nil {
			err = &domain.Fault{Status: "cancelled", Code: "cancelled", Message: "request cancelled before execution"}
		} else {
			switch c := command.(type) {
			case domain.ReadCommand:
				result, err = (application.ReadUseCase{Files: e.files}).Execute(c)
			case domain.WriteCommand:
				result, err = (application.WriteUseCase{Files: e.files, MaxFileBytes: e.config.MaxFileBytes}).Execute(c)
			case domain.EditCommand:
				result, err = (application.EditUseCase{Files: e.files, MaxFileBytes: e.config.MaxFileBytes}).Execute(c)
			case domain.ExecCommand:
				result, err = (application.ExecUseCase{Processes: e.processes}).Execute(ctx, c)
			case domain.SearchCommand:
				listingBytes = int(c.MaxBytes)
				result, err = (application.SearchUseCase{Files: e.files}).Execute(ctx, c)
			case domain.ListCommand:
				listingBytes = int(c.MaxBytes)
				result, err = (application.ListUseCase{Files: e.files}).Execute(ctx, c)
			case domain.PluginListCommand:
				if e.config.Plugins == nil {
					err = domain.Reject("plugins_unavailable", "plugins are not configured")
				} else {
					result, err = (application.ListPluginToolsUseCase{Plugins: e.config.Plugins}).Execute(ctx, c)
				}
			case domain.PluginCall:
				if e.config.Plugins == nil {
					err = domain.Reject("plugins_unavailable", "plugins are not configured")
				} else {
					result, err = (application.CallPluginToolUseCase{Plugins: e.config.Plugins}).Execute(ctx, c)
				}
			}
		}
	} else {
		err = domain.Reject("invalid_request", err.Error())
	}
	if err != nil {
		var fault *domain.Fault
		if errors.Is(err, context.DeadlineExceeded) {
			r.Status = "timed_out"
			r.Error = &dto.Failure{Code: "timeout", Message: "plugin request timed out; prior effects may remain"}
		} else if errors.Is(err, context.Canceled) {
			r.Status = "cancelled"
			r.Error = &dto.Failure{Code: "cancelled", Message: "plugin request cancelled; prior effects may remain"}
		} else if errors.As(err, &fault) {
			r.Status = fault.Status
			r.Error = &dto.Failure{Code: fault.Code, Message: fault.Message}
		} else {
			r.Status = "failed"
			r.Error = &dto.Failure{Code: "internal_error", Message: err.Error()}
		}
	} else {
		r.Output = (ResponseMapper{ListingBytes: listingBytes}).Map(result)
	}
	r.FinishedAt = time.Now().UTC()
	r.DurationMS = time.Since(start).Milliseconds()
	return r
}
