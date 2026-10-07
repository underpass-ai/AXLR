package diagnostics

import (
	"encoding/json"
	"errors"
	"github.com/underpass-ai/AXLR/tui/application"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

type Transport struct {
	Next     http.RoundTripper
	Trace    application.DiagnosticPort
	Payloads *PayloadRecorder
	// Endpoints are the base URLs of configured local model servers, such as
	// http://127.0.0.1:8080/v1. Their requests are traced like OpenRouter's.
	Endpoints []string
}

// classify reports whether a request goes to a model provider and which
// endpoint it reaches. OpenRouter is always traced; a local server only when
// its scheme, host and port match a configured endpoint and the path lies
// under that endpoint's path.
func (t Transport) classify(target *url.URL) (application.DiagnosticEndpoint, bool) {
	var rest string
	if strings.EqualFold(target.Hostname(), "openrouter.ai") && strings.HasPrefix(target.Path, "/api/v1/") {
		rest = strings.TrimPrefix(target.Path, "/api/v1")
	} else {
		matched := false
		for _, raw := range t.Endpoints {
			base, err := url.Parse(raw)
			if err != nil || !strings.EqualFold(base.Scheme, target.Scheme) || !strings.EqualFold(base.Host, target.Host) {
				continue
			}
			prefix := strings.TrimRight(base.Path, "/")
			if strings.HasPrefix(target.Path, prefix+"/") {
				rest, matched = strings.TrimPrefix(target.Path, prefix), true
				break
			}
		}
		if !matched {
			return "", false
		}
	}
	switch rest {
	case "/chat/completions":
		return application.DiagnosticEndpointChat, true
	case "/models":
		return application.DiagnosticEndpointModels, true
	}
	return application.DiagnosticEndpointOther, true
}

var requestSequence atomic.Uint64

func (t Transport) RoundTrip(request *http.Request) (*http.Response, error) {
	next := t.Next
	if next == nil {
		next = http.DefaultTransport
	}
	endpoint, traced := t.classify(request.URL)
	if !traced {
		return next.RoundTrip(request)
	}
	id := requestSequence.Add(1)
	started := time.Now()
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
				Messages json.RawMessage `json:"messages"`
				Tools    json.RawMessage `json:"tools"`
			}
			if json.Unmarshal(data, &summary) == nil {
				var messages, tools []json.RawMessage
				if json.Unmarshal(summary.Messages, &messages) == nil {
					event.Messages = len(messages)
					event.MessageBytes = len(summary.Messages)
				}
				if json.Unmarshal(summary.Tools, &tools) == nil {
					event.Tools = len(tools)
					event.ToolSchemaBytes = len(summary.Tools)
				}
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
		isSSE := !isError && (strings.HasPrefix(contentType, "text/event-stream") || (contentType == "" && endpoint == application.DiagnosticEndpointChat))
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
