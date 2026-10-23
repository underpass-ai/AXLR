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
	// reasoningBytes is the reasoning streamed so far; reasoningTold what
	// the console was last told of it.
	reasoningBytes, reasoningTold int
	// reasoningText is the readable reasoning streamed since the last
	// report, bounded to what a console shows.
	reasoningText string
}

// reasoningStep is how much more reasoning a stream reports progress
// after: a frame carries a few bytes, and each report redraws the console.
const reasoningStep = 512

// maxReasoningText bounds the reasoning text one report carries: the
// console shows a few lines of it.
const maxReasoningText = 2048

// lastBytes is the end of text, at most limit bytes, cut on a character
// boundary.
func lastBytes(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	cut := len(text) - limit
	for cut < len(text) && text[cut]&0xC0 == 0x80 {
		cut++
	}
	return text[cut:]
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
	if len(frame.Choices) == 0 {
		b.observeNativeFrame(data)
		return
	}
	for _, choice := range frame.Choices {
		if choice.Delta.Content != "" {
			b.observeContent(len(choice.Delta.Content))
		}
		text := choice.Delta.Reasoning + choice.Delta.ReasoningContent
		// Providers may duplicate text and structured reasoning representations.
		if text == "" {
			for _, detail := range choice.Delta.ReasoningDetails {
				text += detail.Text + detail.Summary
			}
		}
		if len(text) > 0 || len(choice.Delta.ReasoningDetails) > 0 {
			b.observeReasoning(text)
		}
		size := 0
		for _, call := range choice.Delta.ToolCalls {
			size += len(call.Function.Name) + len(call.Function.Arguments)
			b.observeToolName(call.Function.Name)
			b.toolBytes += len(call.Function.Arguments)
		}
		if len(choice.Delta.ToolCalls) > 0 {
			b.observeToolCall(size, len(choice.Delta.ToolCalls))
		}
	}
}

// observeNativeFrame reads the events of the Anthropic Messages API
// (content_block_start, content_block_delta with text_delta, thinking_delta,
// input_json_delta) and the OpenAI Responses API (response.output_text.delta,
// response.reasoning_summary_text.delta, response.function_call_arguments.delta,
// response.output_item.added), which carry no choices, for the same phases.
func (b *streamBody) observeNativeFrame(data []byte) {
	var event struct {
		Type         string          `json:"type"`
		Delta        json.RawMessage `json:"delta"`
		ContentBlock struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"content_block"`
		Item struct {
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"item"`
	}
	if json.Unmarshal(data, &event) != nil {
		return
	}
	var text string
	switch event.Type {
	case "content_block_start":
		switch event.ContentBlock.Type {
		case "tool_use":
			b.observeToolName(event.ContentBlock.Name)
			b.observeToolCall(len(event.ContentBlock.Name), 1)
		case "thinking", "redacted_thinking":
			b.observeReasoning("")
		}
	case "content_block_delta":
		var delta struct {
			Type        string `json:"type"`
			Text        string `json:"text"`
			Thinking    string `json:"thinking"`
			PartialJSON string `json:"partial_json"`
		}
		if json.Unmarshal(event.Delta, &delta) != nil {
			return
		}
		switch delta.Type {
		case "text_delta":
			b.observeContent(len(delta.Text))
		case "thinking_delta", "signature_delta":
			b.observeReasoning(delta.Thinking)
		case "input_json_delta":
			b.toolBytes += len(delta.PartialJSON)
			b.observeToolCall(len(delta.PartialJSON), 1)
		}
	case "response.output_item.added":
		switch event.Item.Type {
		case "function_call":
			b.observeToolName(event.Item.Name)
			b.observeToolCall(len(event.Item.Name), 1)
		case "reasoning":
			b.observeReasoning("")
		}
	case "response.output_text.delta", "response.reasoning_summary_text.delta", "response.reasoning_text.delta", "response.function_call_arguments.delta":
		if json.Unmarshal(event.Delta, &text) != nil {
			return
		}
		switch event.Type {
		case "response.output_text.delta":
			b.observeContent(len(text))
		case "response.function_call_arguments.delta":
			b.toolBytes += len(text)
			b.observeToolCall(len(text), 1)
		default:
			b.observeReasoning(text)
		}
	}
}

// observeContent records streamed answer text.
func (b *streamBody) observeContent(size int) {
	b.activity(domain.ProviderContent)
	b.record(application.DiagnosticContent, size, 1)
}

// observeReasoning records streamed reasoning and tells the console of it
// every reasoningStep bytes; text may be empty for a structured frame.
func (b *streamBody) observeReasoning(text string) {
	size := len(text)
	b.activity(domain.ProviderReasoning)
	b.record(application.DiagnosticReasoning, size, 1)
	b.reasoningBytes += size
	b.reasoningText = lastBytes(b.reasoningText+text, maxReasoningText)
	if b.reasoningBytes-b.reasoningTold >= reasoningStep || b.reasoningTold == 0 && b.reasoningBytes > 0 {
		b.reasoningTold = b.reasoningBytes
		application.NotifyReasoningProgress(b.ctx, b.reasoningBytes, b.reasoningText)
		b.reasoningText = ""
	}
}

// observeToolName starts counting a new call's arguments when a name arrives.
func (b *streamBody) observeToolName(name string) {
	if name != "" && name != b.toolName {
		b.toolName, b.toolBytes = name, 0
	}
}

// observeToolCall records a tool-call fragment and tells the console how far
// the current call's arguments have come.
func (b *streamBody) observeToolCall(size, count int) {
	b.activity(domain.ProviderToolCall)
	application.NotifyToolCallProgress(b.ctx, b.toolName, b.toolBytes)
	b.record(application.DiagnosticToolDelta, size, count)
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
			// A read error, a failed drain or a cancellation cuts the body
			// short; wire_done and the HTTP span already carry that class.
			// The capture of what arrived fails only when it is not saved.
			var saveErr error
			if b.oversized {
				saveErr = io.ErrShortBuffer
			} else {
				saveErr = b.payloads.SaveResponse(b.id, b.body, !b.jsonResponse)
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

// errorDrainTimeout bounds the read of an error body the client left unread;
// past it the capture keeps what arrived.
var errorDrainTimeout = 500 * time.Millisecond

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
	ctx, cancel := context.WithTimeout(ctx, errorDrainTimeout)
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
