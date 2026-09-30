package diagnostics

import (
	"encoding/json"
	"errors"
	"github.com/underpass-ai/AXLR/tui/application"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

type Transport struct {
	Next     http.RoundTripper
	Trace    application.DiagnosticPort
	Payloads *PayloadRecorder
}

var requestSequence atomic.Uint64

func (t Transport) RoundTrip(request *http.Request) (*http.Response, error) {
	next := t.Next
	if next == nil {
		next = http.DefaultTransport
	}
	if !strings.EqualFold(request.URL.Hostname(), "openrouter.ai") || !strings.HasPrefix(request.URL.Path, "/api/v1/") {
		return next.RoundTrip(request)
	}
	id := requestSequence.Add(1)
	started := time.Now()
	endpoint := application.DiagnosticEndpointOther
	if request.URL.Path == "/api/v1/chat/completions" {
		endpoint = application.DiagnosticEndpointChat
	}
	if request.URL.Path == "/api/v1/models" {
		endpoint = application.DiagnosticEndpointModels
	}
	event := application.DiagnosticEvent{Stage: application.DiagnosticRequestSent, Endpoint: endpoint, RequestID: id, Bytes: max(0, int(request.ContentLength))}
	ctx, span := application.StartDiagnosticSpan(request.Context(), t.Trace, application.DiagnosticActionHTTP, event)
	request = request.WithContext(ctx)
	event.SpanID = application.CurrentDiagnosticSpan(ctx)
	if t.Payloads != nil || t.Trace != nil {
		_, captureSpan := application.StartDiagnosticSpan(ctx, t.Trace, application.DiagnosticActionPayload, application.DiagnosticEvent{Endpoint: endpoint, RequestID: id, Bytes: event.Bytes})
		var data []byte
		var captureErr error
		if request.Body == nil || request.Body == http.NoBody {
			data = nil
		} else if request.GetBody == nil {
			captureErr = errors.New("request body cannot be cloned")
		} else {
			var body io.ReadCloser
			body, captureErr = request.GetBody()
			if captureErr == nil {
				data, captureErr = io.ReadAll(io.LimitReader(body, payloadLimit+1))
				_ = body.Close()
				if len(data) > payloadLimit {
					captureErr = io.ErrShortBuffer
				}
			}
		}
		if captureErr == nil {
			var summary struct {
				Messages []json.RawMessage `json:"messages"`
				Tools    []json.RawMessage `json:"tools"`
			}
			if json.Unmarshal(data, &summary) == nil {
				event.Messages = len(summary.Messages)
				event.Tools = len(summary.Tools)
			}
		}
		if t.Payloads != nil {
			if captureErr == nil {
				captureErr = t.Payloads.Save(id, "request", data)
			}
			if t.Trace != nil {
				stage, class := application.DiagnosticPayloadSaved, application.DiagnosticErrorNone
				if captureErr != nil {
					stage, class = application.DiagnosticPayloadFailed, application.DiagnosticErrorStorage
				}
				_ = t.Trace.Record(application.DiagnosticEvent{Stage: stage, Endpoint: endpoint, SpanID: event.SpanID, RequestID: id, Bytes: len(data), ErrorClass: class})
			}
		}
		captureClass := application.DiagnosticErrorNone
		if captureErr != nil {
			captureClass = application.DiagnosticErrorStorage
		}
		captureSpan.End(captureClass)
	}
	if t.Trace != nil {
		_ = t.Trace.Record(event)
	}
	response, err := next.RoundTrip(request)
	status := 0
	if response != nil {
		status = response.StatusCode
	}
	if t.Trace != nil {
		class := application.DiagnosticErrorNone
		if err != nil || status >= 400 || response == nil {
			class = application.DiagnosticErrorProvider
		}
		_ = t.Trace.Record(application.DiagnosticEvent{Stage: application.DiagnosticProviderHeaders, Endpoint: endpoint, SpanID: event.SpanID, RequestID: id, Bytes: max(0, int(request.ContentLength)), HTTPStatus: status, ElapsedMilliseconds: time.Since(started).Milliseconds(), ErrorClass: class})
	}
	if err == nil && response != nil && response.Body != nil {
		isError := status < 200 || status >= 300
		contentType := strings.ToLower(response.Header.Get("Content-Type"))
		isSSE := !isError && (strings.HasPrefix(contentType, "text/event-stream") || (contentType == "" && request.URL.Path == "/api/v1/chat/completions"))
		response.Body = &streamBody{next: response.Body, trace: t.Trace, payloads: t.Payloads, endpoint: endpoint, id: id, started: started, ctx: ctx, span: span, errorResponse: isError, jsonResponse: !isSSE}
	} else {
		class := application.DiagnosticErrorNone
		if err != nil || response == nil || status < 200 || status >= 300 {
			class = diagnosticHTTPError(err)
		}
		if t.Payloads != nil && t.Trace != nil {
			_ = t.Trace.Record(application.DiagnosticEvent{Stage: application.DiagnosticPayloadFailed, Endpoint: endpoint, SpanID: event.SpanID, RequestID: id, ErrorClass: class})
		}
		span.End(class)
	}
	return response, err
}
