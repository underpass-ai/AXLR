package openrouter

import (
	"context"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/underpass-ai/AXLR/domain"
)

// relaceUsage is the usage OpenRouter sent in the last chunk of a
// z-ai/glm-5.3-flash stream on 9 October 2026, which Relace served.
const relaceUsage = `{"prompt_tokens":7448,"completion_tokens":2567,"total_tokens":10015,"cost":0.00158142,"is_byok":false,` +
	`"prompt_tokens_details":{"cached_tokens":0,"cache_write_tokens":0,"audio_tokens":0},` +
	`"cost_details":{"upstream_inference_cost":0.00158142,"upstream_inference_prompt_cost":0.00029792,"upstream_inference_completions_cost":0.0012835},` +
	`"completion_tokens_details":{"reasoning_tokens":2590,"image_tokens":0}}`

func sameDollars(a, b float64) bool { return math.Abs(a-b) < 1e-12 }

// Every chunk names the provider and the last one carries the usage with
// its cost; the stream is read one byte at a time so both cross reads.
func TestStreamReadsCostReasoningTokensAndProvider(t *testing.T) {
	wire := streamEvents(
		`{"id":"gen-1","provider":"Relace","model":"z-ai/glm-5.3-flash","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"role":"assistant","content":"hé"},"finish_reason":null}]}`,
		`{"id":"gen-1","provider":"Relace","model":"z-ai/glm-5.3-flash","object":"chat.completion.chunk","choices":[{"index":0,"delta":{"content":"llo"},"finish_reason":"stop"}]}`,
		`{"id":"gen-1","provider":"Relace","model":"z-ai/glm-5.3-flash","object":"chat.completion.chunk","choices":[],"usage":`+relaceUsage+`}`,
		"[DONE]")
	c, _ := streamClient(t, &trackedStreamBody{Reader: iotest.OneByteReader(strings.NewReader(wire))}, 200)
	got, err := c.Stream(context.Background(), simpleCompletionRequest(), func(domain.Text) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	usage := got.Usage
	if usage == nil || !usage.CostKnown || !sameDollars(usage.Cost, 0.00158142) || usage.ReasoningTokens != 2590 || usage.PromptTokens != 7448 || usage.CompletionTokens != 2567 || usage.CachedTokens != 0 {
		t.Fatalf("usage = %+v", usage)
	}
	if got.Provider != "Relace" || got.Message.Content != "héllo" {
		t.Fatalf("provider %q, content %q", got.Provider, got.Message.Content)
	}
}

// A whole reply carries the provider and the usage at its top level too.
func TestCompleteReadsCostAndProvider(t *testing.T) {
	client, err := New(ClientConfig{APIKey: "test-secret", HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		_, _ = io.Copy(io.Discard, req.Body)
		return testResponse(200, `{"provider":"InferenceNet","choices":[{"message":{"role":"assistant","content":"Hello"},"finish_reason":"stop"}],"usage":`+relaceUsage+`}`), nil
	})}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.Complete(context.Background(), simpleCompletionRequest())
	if err != nil || got.Provider != "InferenceNet" || got.Usage == nil || !got.Usage.CostKnown || !sameDollars(got.Usage.Cost, 0.00158142) || got.Usage.ReasoningTokens != 2590 {
		t.Fatalf("completion = %+v, usage %+v, %v", got, got.Usage, err)
	}
}

// A local server reports tokens without a cost: the cost stays unknown,
// never $0. A request with the person's own key costs OpenRouter's fee
// plus what the provider billed to that key; without one,
// upstream_inference_cost repeats cost and is not added.
func TestUsageCostKnownOnlyWhenReported(t *testing.T) {
	for _, tc := range []struct {
		name  string
		usage string
		known bool
		cost  float64
	}{
		{"local server", `{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}`, false, 0},
		{"free model", `{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,"cost":0}`, true, 0},
		{"openrouter credits", relaceUsage, true, 0.00158142},
		{"own key", `{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,"cost":0.0001,"is_byok":true,"cost_details":{"upstream_inference_cost":0.002}}`, true, 0.0021},
	} {
		wire, _ := mapFixture(t, `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":`+tc.usage+`}`)
		got, err := mapResponse(wire)
		if err != nil {
			t.Fatal(err)
		}
		if got.Usage.CostKnown != tc.known || !sameDollars(got.Usage.Cost, tc.cost) || got.Provider != "" {
			t.Fatalf("%s: usage %+v provider %q", tc.name, got.Usage, got.Provider)
		}
	}
}
