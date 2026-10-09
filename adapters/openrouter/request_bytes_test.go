package openrouter

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/underpass-ai/AXLR/domain"
)

// The console learns how many bytes a prompt token holds from the body it
// sent and the prompt tokens the provider counted, so both results report
// the exact size of the body that went out.
func TestResultsReportTheSizeOfTheRequestBody(t *testing.T) {
	for _, stream := range []bool{false, true} {
		sent := 0
		client, err := New(ClientConfig{APIKey: "secret", HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatal(err)
			}
			sent = len(raw)
			if stream {
				return testResponse(200, streamEvents(textChunk, stopChunk, `{"choices":[],"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}}`, "[DONE]")), nil
			}
			return testResponse(200, `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":1,"total_tokens":4}}`), nil
		})}})
		if err != nil {
			t.Fatal(err)
		}
		var got domain.CompletionResult
		if stream {
			got, err = client.Stream(context.Background(), simpleCompletionRequest(), func(domain.Text) error { return nil })
		} else {
			got, err = client.Complete(context.Background(), simpleCompletionRequest())
		}
		if err != nil {
			t.Fatal(err)
		}
		if sent == 0 || got.RequestBytes != sent {
			t.Fatalf("stream=%v: RequestBytes = %d, body sent = %d", stream, got.RequestBytes, sent)
		}
	}
}
