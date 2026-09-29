package openrouter

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type catalogRoundTrip func(*http.Request) (*http.Response, error)

func (f catalogRoundTrip) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func catalogResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func TestModelCatalogListsValidModels(t *testing.T) {
	requests := 0
	client := &http.Client{Transport: catalogRoundTrip(func(req *http.Request) (*http.Response, error) {
		requests++
		if req.Method != http.MethodGet || req.URL.String() != "https://openrouter.ai/api/v1/models?supported_parameters=tools&output_modalities=text" {
			t.Errorf("unexpected request: %s %s", req.Method, req.URL)
		}
		if got := req.Header.Get("Authorization"); got != "Bearer test-secret" {
			t.Errorf("Authorization = %q", got)
		}
		if req.Body != nil {
			t.Error("catalog request must not contain a completion body")
		}
		return catalogResponse(200, `{"data":[{"id":"provider/one","name":"One","context_length":128000,"pricing":{"prompt":"0.000001","completion":"0.000002"},"supported_parameters":["tools","temperature"],"architecture":{"output_modalities":["text"]}},{"id":"provider/two","name":"Two","supported_parameters":["tools"],"architecture":{"output_modalities":["text"]}}]}`), nil
	})}
	models, err := (ModelCatalog{APIKey: "test-secret", HTTPClient: client}).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
	if len(models) != 2 {
		t.Fatalf("models = %#v", models)
	}
	if string(models[0].ID) != "provider/one" || string(models[0].Name) != "One" || models[0].Context.Tokens() != 128000 || models[0].PromptRate.Display() != "$1 / 1M tokens" || models[0].CompletionRate.Display() != "$2 / 1M tokens" || !models[0].SupportsTools || !models[0].TextOutput {
		t.Errorf("mapped model = %#v", models[0])
	}
	if models[1].Context.Tokens() != 0 || models[1].PromptRate.Display() != "" {
		t.Errorf("unknown metadata = %#v", models[1])
	}
}

func TestModelCatalogSkipsInvalidEntriesAndOptionalMetadata(t *testing.T) {
	body := `{"data":[{"id":"valid/one","name":"Valid","context_length":"bad","pricing":{"prompt":"-1","completion":"0.000003"},"supported_parameters":["tools"],"architecture":{"output_modalities":["text"]}},{"id":"","name":"Empty ID","supported_parameters":["tools"],"architecture":{"output_modalities":["text"]}},{"id":"bad/name","name":"\u001b[31mBad","supported_parameters":["tools"],"architecture":{"output_modalities":["text"]}},{"id":"no/tools","name":"No Tools","supported_parameters":[],"architecture":{"output_modalities":["text"]}},{"id":"no/text","name":"No Text","supported_parameters":["tools"],"architecture":{"output_modalities":["image"]}},{"id":"missing/capabilities","name":"Missing"},{"id":"malformed/capabilities","name":"Malformed","supported_parameters":"tools","architecture":{"output_modalities":["text"]}},42]}`
	client := &http.Client{Transport: catalogRoundTrip(func(*http.Request) (*http.Response, error) { return catalogResponse(200, body), nil })}
	models, err := (ModelCatalog{APIKey: "key", HTTPClient: client}).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || string(models[0].ID) != "valid/one" || models[0].Context.Tokens() != 0 || models[0].PromptRate.Display() != "" || models[0].CompletionRate.Display() != "$3 / 1M tokens" {
		t.Errorf("models = %#v", models)
	}
}

func TestModelCatalogRejectsProviderFailuresWithoutLeakingSecrets(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			requests := 0
			client := &http.Client{Transport: catalogRoundTrip(func(*http.Request) (*http.Response, error) {
				requests++
				return catalogResponse(status, `{"error":"test-secret sensitive-body"}`), nil
			})}
			_, err := (ModelCatalog{APIKey: "test-secret", HTTPClient: client}).List(context.Background())
			if err == nil || strings.Contains(err.Error(), "test-secret") || strings.Contains(err.Error(), "sensitive-body") || requests != 1 {
				t.Errorf("requests=%d error=%v", requests, err)
			}
		})
	}
}

func TestModelCatalogRejectsMalformedEnvelopeAndOversizedBody(t *testing.T) {
	for _, body := range []string{`{"data":{}}`, `{"error":"no data"}`, `{"data":[`, `{"data":null}`, strings.Repeat("x", 8<<20+1)} {
		client := &http.Client{Transport: catalogRoundTrip(func(*http.Request) (*http.Response, error) { return catalogResponse(200, body), nil })}
		_, err := (ModelCatalog{APIKey: "key", HTTPClient: client}).List(context.Background())
		if err == nil {
			t.Errorf("accepted malformed or oversized body of %d bytes", len(body))
		}
	}
}

func TestModelCatalogHonorsCancellationAndTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	requests := 0
	client := &http.Client{Transport: catalogRoundTrip(func(req *http.Request) (*http.Response, error) {
		requests++
		<-req.Context().Done()
		return nil, req.Context().Err()
	})}
	_, err := (ModelCatalog{APIKey: "key", HTTPClient: client}).List(ctx)
	if !errors.Is(err, context.Canceled) || requests != 0 {
		t.Errorf("canceled: requests=%d error=%v", requests, err)
	}
	_, err = (ModelCatalog{APIKey: "key", HTTPClient: client}).List(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) || requests != 1 {
		t.Errorf("timeout: requests=%d error=%v", requests, err)
	}
}

func TestModelCatalogBoundsRequestDeadline(t *testing.T) {
	client := &http.Client{Timeout: time.Minute, Transport: catalogRoundTrip(func(req *http.Request) (*http.Response, error) {
		deadline, ok := req.Context().Deadline()
		if !ok || time.Until(deadline) > 10*time.Second || time.Until(deadline) < 9*time.Second {
			t.Errorf("request deadline = %v", deadline)
		}
		return catalogResponse(200, `{"data":[]}`), nil
	})}
	_, err := (ModelCatalog{APIKey: "key", HTTPClient: client}).List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
}
