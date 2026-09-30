package diagnostics

import (
	"net/http"
	"time"

	"github.com/underpass-ai/AXLR/tui/application"
)

type Transport struct {
	Next  http.RoundTripper
	Trace application.DiagnosticPort
}

func (t Transport) RoundTrip(request *http.Request) (*http.Response, error) {
	next := t.Next
	if next == nil {
		next = http.DefaultTransport
	}
	started := time.Now()
	response, err := next.RoundTrip(request)
	if t.Trace != nil && request.URL.Path == "/api/v1/chat/completions" {
		class := application.DiagnosticErrorNone
		if err != nil || response == nil || response.StatusCode >= 400 {
			class = application.DiagnosticErrorProvider
		}
		_ = t.Trace.Record(application.DiagnosticEvent{Stage: application.DiagnosticProviderHeaders, Bytes: int(request.ContentLength), ElapsedMilliseconds: time.Since(started).Milliseconds(), ErrorClass: class})
	}
	return response, err
}
