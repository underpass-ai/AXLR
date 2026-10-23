package provider

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
)

// The decoder joins data lines and skips event names and comments, as every
// provider's stream carries its type in the data.
func TestSSEDecoderSkipsEventNamesAndComments(t *testing.T) {
	stream := "event: message_start\ndata: {\"type\":\"message_start\"}\n\n: keep-alive\n\nevent: x\ndata: {\"a\":\ndata: 1}\r\n\r\ndata: [DONE]\n\n"
	decoder := NewSSEDecoder(strings.NewReader(stream), "model endpoint x")
	var events []string
	for {
		data, err := decoder.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		events = append(events, string(data))
	}
	if strings.Join(events, "|") != `{"type":"message_start"}|{"a":`+"\n"+`1}|[DONE]` {
		t.Fatalf("events %q", events)
	}
	if _, err := NewSSEDecoder(strings.NewReader("data: cut"), "").Next(); err != io.ErrUnexpectedEOF {
		t.Fatalf("cut event: %v", err)
	}
	half := strings.Repeat("x", MaxSSEEventBytes/2)
	_, err := NewSSEDecoder(strings.NewReader("data: "+half+"\ndata: "+half+"\n\n"), "model endpoint x").Next()
	if err == nil || err.Error() != "model endpoint x SSE event exceeds 1 MiB" {
		t.Fatalf("oversized: %v", err)
	}
	if _, err := NewSSEDecoder(strings.NewReader("data: "+strings.Repeat("x", 2*MaxSSEEventBytes)+"\n\n"), "").Next(); err == nil {
		t.Fatal("oversized line accepted")
	}
}

func TestClassifyProviderErrorAndHints(t *testing.T) {
	for status, category := range map[int]ProviderErrorCategory{400: CategoryInvalidRequest, 401: CategoryAuthentication, 402: CategoryInsufficientCredits, 404: CategoryInvalidRequest, 429: CategoryRateLimited, 529: CategoryRateLimited, 500: CategoryProviderFailure} {
		if got := ClassifyProviderError(status); got.Category != category || got.StatusCode != status {
			t.Fatalf("HTTP %d: %+v", status, got)
		}
	}
	for message, hint := range map[string]ProviderHint{
		"model: claude-nonexistent":                  HintChooseModel,
		"anthropic/claude-x is not a valid model ID": HintChooseModel,
		"The model `gpt-9` does not exist":           HintChooseModel,
		"max_tokens: too large":                      HintNone,
		"messages.5: tool_result without a tool_use": HintNone,
	} {
		err := ClassifyProviderError(400)
		err.Message = message
		if err.HintKind() != hint {
			t.Fatalf("%q: hint %q", message, err.HintKind())
		}
	}
	local := &ProviderError{StatusCode: 402, Category: CategoryInsufficientCredits, Provider: "model endpoint api.openai.com"}
	if strings.Contains(local.Hint(), "openrouter") || local.Error() != "model endpoint api.openai.com insufficient_credits (HTTP 402)" {
		t.Fatalf("local credits: %q %q", local.Hint(), local.Error())
	}
	if (&ProviderError{StatusCode: 402, Category: CategoryInsufficientCredits}).Error() != "OpenRouter insufficient_credits (HTTP 402)" {
		t.Fatal("empty provider is not OpenRouter")
	}
}

// Error bodies: a numeric code is the status, a string code is not, and the
// string code is read beside the message.
func TestErrorBodiesAreRead(t *testing.T) {
	if status, message, ok := ErrorBody(json.RawMessage(`{"code":503,"message":"over"}`)); !ok || status != 503 || message != "over" {
		t.Fatalf("numeric code: %d %q %v", status, message, ok)
	}
	if status, message, ok := ErrorBody(json.RawMessage(`{"code":"insufficient_quota","message":"quota"}`)); !ok || status != 502 || message != "quota" {
		t.Fatalf("string code: %d %q %v", status, message, ok)
	}
	if _, _, ok := ErrorBody(json.RawMessage(`null`)); ok {
		t.Fatal("null body read as an error")
	}
	message, code := ReadErrorResponse(strings.NewReader(`{"error":{"message":"You exceeded your quota\nbearer abcdef","type":"insufficient_quota","code":"insufficient_quota"}}`))
	if message != "You exceeded your quota [redacted]" || code != "insufficient_quota" {
		t.Fatalf("%q %q", message, code)
	}
	message, code = ReadErrorResponse(strings.NewReader(`{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`))
	if message != "Overloaded" || code != "overloaded_error" {
		t.Fatalf("%q %q", message, code)
	}
	if message, code := ReadErrorResponse(strings.NewReader(`<html>`)); message != "" || code != "" {
		t.Fatalf("%q %q", message, code)
	}
}

// Broken arguments become the malformed-arguments object the console refuses
// with the reason; empty arguments are the empty object.
func TestToolArgumentsRepairsWhatIsNotAnObject(t *testing.T) {
	if got, err := ToolArguments("  ", false); err != nil || string(got.Bytes()) != "{}" {
		t.Fatalf("empty: %s %v", got.Bytes(), err)
	}
	if got, err := ToolArguments(`{"path":"a"}`, false); err != nil || string(got.Bytes()) != `{"path":"a"}` {
		t.Fatalf("object: %s %v", got.Bytes(), err)
	}
	got, err := ToolArguments(`{"path":"a","content":"unfin`, true)
	if err != nil {
		t.Fatal(err)
	}
	var repaired map[string]struct {
		Error            string `json:"error"`
		RawPrefix        string `json:"raw_prefix"`
		CutByOutputLimit bool   `json:"cut_by_output_limit"`
	}
	if err := json.Unmarshal(got.Bytes(), &repaired); err != nil {
		t.Fatal(err)
	}
	malformed, ok := repaired[MalformedArgumentsKey]
	if !ok || !malformed.CutByOutputLimit || malformed.RawPrefix != `{"path":"a","content":"unfin` || malformed.Error == "" {
		t.Fatalf("repaired %+v", repaired)
	}
	got, _ = ToolArguments(`[1,2]`, false)
	if !strings.Contains(string(got.Bytes()), "arguments must be a JSON object") {
		t.Fatalf("array: %s", got.Bytes())
	}
}

func TestEndpoints(t *testing.T) {
	if !IsLoopbackEndpoint("http://127.0.0.1:8080/v1") || !IsLoopbackEndpoint("http://localhost/v1") || !IsLoopbackEndpoint("http://[::1]:8/v1") || IsLoopbackEndpoint("https://api.anthropic.com") {
		t.Fatal("loopback")
	}
	if ValidateEndpoint("https://api.anthropic.com/v1/messages") != nil || ValidateEndpoint("http://127.0.0.1:1/v1/messages") != nil {
		t.Fatal("valid endpoints refused")
	}
	for _, raw := range []string{"http://api.anthropic.com/v1/messages", "https://u:p@x.example/v1", "https://x.example/v1?x=1", "api.openai.com/v1", ""} {
		if ValidateEndpoint(raw) == nil {
			t.Fatalf("%q accepted", raw)
		}
	}
	if EndpointName("https://api.openai.com/v1/responses") != "model endpoint api.openai.com" || EndpointName("::") != "model endpoint" || EndpointHost("https://api.openai.com/v1") != "api.openai.com" {
		t.Fatal("names")
	}
}
