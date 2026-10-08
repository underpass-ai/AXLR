package diagnostics

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
	"io"
	"sync"
	"time"
)

// streamBody observes bytes without altering the provider's stream or callbacks.
// Non-text frames are measured separately from visible answer text.
type streamBody struct {
	phase           domain.ProviderPhase
	next            io.ReadCloser
	trace           application.DiagnosticPort
	payloads        *PayloadRecorder
	id              uint64
	started         time.Time
	mu              sync.Mutex
	readMu          sync.Mutex
	closed          bool
	closeOnce       sync.Once
	closeErr        error
	underlyingClose sync.Once
	underlyingErr   error
	lastCR          bool
	reads           int
	frames          int
	bytes           int
	line            []byte
	data            []byte
	body            []byte
	oversized       bool
	ctx             context.Context
	span            *application.DiagnosticSpan
	endpoint        application.DiagnosticEndpoint
	errorResponse   bool
	jsonResponse    bool
	readErr         error
	drainErr        error
	toolName        string
	toolBytes       int
}

func (b *streamBody) record(stage application.DiagnosticStage, size, count int) {
	if b.trace != nil {
		_ = b.trace.Record(application.DiagnosticEvent{Stage: stage, Endpoint: b.endpoint, SpanID: application.CurrentDiagnosticSpan(b.ctx), RequestID: b.id, Bytes: size, Chunks: count, ElapsedMilliseconds: time.Since(b.started).Milliseconds()})
	}
}

func (b *streamBody) activity(phase domain.ProviderPhase) {
	// Text is final for this stream's status. A late duplicated reasoning frame
	// must not turn a visible answer back into a waiting indicator.
	if b.phase == domain.ProviderContent || b.phase == phase {
		return
	}
	b.phase = phase
	application.NotifyProviderActivity(b.ctx, phase)
}
func (b *streamBody) Read(p []byte) (int, error) {
	b.readMu.Lock()
	defer b.readMu.Unlock()
	n, err := b.next.Read(p)
	b.observeRead(p[:n], err)
	return n, err
}

func (b *streamBody) observeRead(p []byte, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	if err != nil && err != io.EOF {
		b.readErr = err
	}
	n := len(p)
	if n == 0 {
		return
	}
	b.reads++
	b.bytes += n
	b.record(application.DiagnosticWireBytes, n, b.reads)
	if b.payloads != nil && !b.oversized {
		if len(b.body)+n > payloadLimit {
			b.oversized = true
			b.body = nil
		} else {
			b.body = append(b.body, p...)
		}
	}
	if b.jsonResponse {
		return
	}
	for _, c := range p {
		switch c {
		case '\r':
			b.observeLine(b.line)
			b.line = b.line[:0]
			b.lastCR = true
		case '\n':
			if !b.lastCR {
				b.observeLine(b.line)
				b.line = b.line[:0]
			}
			b.lastCR = false
		default:
			b.lastCR = false
			if len(b.line) < 1024*1024 {
				b.line = append(b.line, c)
			}
		}
	}
}
func (b *streamBody) observeLine(line []byte) {
	if len(line) == 0 {
		if len(b.data) > 0 {
			b.observeFrame(b.data)
			b.data = b.data[:0]
		}
		return
	}
	if line[0] == ':' {
		b.record(application.DiagnosticHeartbeat, len(line), 1)
		return
	}
	if bytes.HasPrefix(line, []byte("data:")) {
		value := bytes.TrimPrefix(line[5:], []byte{' '})
		if len(b.data)+len(value)+1 > 1024*1024 {
			b.data = b.data[:0]
			return
		}
		if len(b.data) > 0 {
			b.data = append(b.data, '\n')
		}
		b.data = append(b.data, value...)
	}
}
func (b *streamBody) observeFrame(data []byte) {
	b.frames++
	b.record(application.DiagnosticFrame, len(data), b.frames)
	var frame struct {
		Choices []struct {
			Delta struct {
				Content          string `json:"content"`
				Reasoning        string `json:"reasoning"`
				ReasoningContent string `json:"reasoning_content"`
				ReasoningDetails []struct {
					Text    string `json:"text"`
					Summary string `json:"summary"`
				} `json:"reasoning_details"`
				ToolCalls []struct {
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if json.Unmarshal(data, &frame) != nil {
		return
	}
	for _, choice := range frame.Choices {
		if choice.Delta.Content != "" {
			b.activity(domain.ProviderContent)
			b.record(application.DiagnosticContent, len(choice.Delta.Content), 1)
		}
		size := len(choice.Delta.Reasoning) + len(choice.Delta.ReasoningContent)
		// Providers may duplicate text and structured reasoning representations.
		if size == 0 {
			for _, detail := range choice.Delta.ReasoningDetails {
				size += len(detail.Text) + len(detail.Summary)
			}
		}
		if size > 0 || len(choice.Delta.ReasoningDetails) > 0 {
			b.activity(domain.ProviderReasoning)
			b.record(application.DiagnosticReasoning, size, 1)
		}
		size = 0
		for _, call := range choice.Delta.ToolCalls {
			size += len(call.Function.Name) + len(call.Function.Arguments)
			if call.Function.Name != "" && call.Function.Name != b.toolName {
				b.toolName, b.toolBytes = call.Function.Name, 0
			}
			b.toolBytes += len(call.Function.Arguments)
		}
		if len(choice.Delta.ToolCalls) > 0 {
			b.activity(domain.ProviderToolCall)
			application.NotifyToolCallProgress(b.ctx, b.toolName, b.toolBytes)
			b.record(application.DiagnosticToolDelta, size, len(choice.Delta.ToolCalls))
		}
	}
}
func (b *streamBody) Close() error {
	b.closeOnce.Do(func() {
		if b.errorResponse && b.payloads != nil {
			b.drainErrorResponse()
		}
		b.closeUnderlying()
		b.closeErr = b.underlyingErr
		b.mu.Lock()
		defer b.mu.Unlock()
		b.closed = true
		class := application.DiagnosticErrorNone
		if b.errorResponse || b.readErr != nil || b.drainErr != nil || b.closeErr != nil {
			class = diagnosticHTTPError(errors.Join(b.readErr, b.drainErr, b.closeErr))
		}
		var cancellation error
		if b.ctx != nil {
			cancellation = b.ctx.Err()
		}
		if cancellation != nil {
			class = diagnosticHTTPError(cancellation)
		}
		if b.trace != nil {
			_ = b.trace.Record(application.DiagnosticEvent{Stage: application.DiagnosticWireDone, Endpoint: b.endpoint, SpanID: application.CurrentDiagnosticSpan(b.ctx), RequestID: b.id, Bytes: b.bytes, Chunks: b.frames, ElapsedMilliseconds: time.Since(b.started).Milliseconds(), ErrorClass: class})
		}
		if b.payloads != nil {
			_, captureSpan := application.StartDiagnosticSpan(b.ctx, b.trace, application.DiagnosticActionPayload, application.DiagnosticEvent{Endpoint: b.endpoint, RequestID: b.id, Bytes: b.bytes})
			var saveErr error
			if b.oversized {
				saveErr = io.ErrShortBuffer
			} else {
				saveErr = b.payloads.SaveResponse(b.id, b.body, !b.jsonResponse)
			}
			if saveErr == nil {
				saveErr = errors.Join(b.readErr, b.drainErr, cancellation)
			}
			if b.trace != nil {
				stage := application.DiagnosticPayloadSaved
				class := application.DiagnosticErrorNone
				if saveErr != nil {
					stage = application.DiagnosticPayloadFailed
					class = application.DiagnosticErrorStorage
				}
				_ = b.trace.Record(application.DiagnosticEvent{Stage: stage, Endpoint: b.endpoint, SpanID: application.CurrentDiagnosticSpan(b.ctx), RequestID: b.id, Bytes: b.bytes, ErrorClass: class})
			}
			captureClass := application.DiagnosticErrorNone
			if saveErr != nil {
				captureClass = application.DiagnosticErrorStorage
			}
			captureSpan.End(captureClass)
		}
		b.span.End(class)
	})
	return b.closeErr
}

func (b *streamBody) closeUnderlying() {
	b.underlyingClose.Do(func() { b.underlyingErr = b.next.Close() })
}

// Error responses are normally rejected before the provider client reads them.
// Only this path drains on Close; streaming replies retain their normal flow.
func (b *streamBody) drainErrorResponse() {
	_, span := application.StartDiagnosticSpan(b.ctx, b.trace, application.DiagnosticActionErrorDrain, application.DiagnosticEvent{RequestID: b.id, Endpoint: b.endpoint})
	defer func() {
		class := application.DiagnosticErrorNone
		if b.drainErr != nil {
			class = diagnosticHTTPError(b.drainErr)
		}
		span.End(class)
	}()
	ctx := b.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	stop := context.AfterFunc(ctx, b.closeUnderlying)
	defer func() { stop(); cancel() }()
	b.readMu.Lock()
	defer b.readMu.Unlock()
	remaining := payloadLimit
	buffer := make([]byte, 4096)
	for remaining > 0 {
		n, err := b.next.Read(buffer[:min(len(buffer), remaining)])
		b.observeRead(buffer[:n], err)
		remaining -= n
		if err != nil {
			if err != io.EOF {
				b.drainErr = err
			}
			if ctx.Err() != nil {
				b.drainErr = ctx.Err()
			}
			return
		}
		if ctx.Err() != nil {
			b.drainErr = ctx.Err()
			return
		}
	}
	b.drainErr = io.ErrShortBuffer
}

func diagnosticHTTPError(err error) application.DiagnosticErrorClass {
	if errors.Is(err, context.Canceled) {
		return application.DiagnosticErrorCancelled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return application.DiagnosticErrorTimeout
	}
	return application.DiagnosticErrorProvider
}
