package copilot

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	sdk "github.com/github/copilot-sdk/go"
	"github.com/github/copilot-sdk/go/rpc"
	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

// fakeSession plays a session's events, in order, as the SDK delivers them.
func fakeSession(data ...rpc.SessionEventData) chan sdk.SessionEvent {
	events := make(chan sdk.SessionEvent, len(data)+1)
	for _, each := range data {
		events <- sdk.SessionEvent{Data: each}
	}
	return events
}

func delta(message, text string) *rpc.AssistantMessageDeltaData {
	return &rpc.AssistantMessageDeltaData{MessageID: message, DeltaContent: text}
}

func collectText(texts *[]string) func(root.Text) error {
	return func(text root.Text) error {
		*texts = append(*texts, string(text))
		return nil
	}
}

// Each delta reaches onText as it arrives, in order; the final message
// adds only what the deltas did not carry, and the result is the whole
// answer once.
func TestCopilotStreamsDeltasOnceAndInOrder(t *testing.T) {
	tokens := int64(12)
	events := fakeSession(
		&rpc.AssistantReasoningDeltaData{ReasoningID: "r1", DeltaContent: "thinking"},
		delta("m1", "Hel"), delta("m1", ""), delta("m1", "lo"),
		&rpc.AssistantMessageData{MessageID: "m1", Content: "Hello, world"},
		&rpc.AssistantUsageData{InputTokens: &tokens, OutputTokens: &tokens},
		&rpc.SessionIdleData{},
	)
	var texts []string
	var reasoning []string
	ctx := application.WithReasoningProgress(context.Background(), func(bytes int, text string) { reasoning = append(reasoning, text) })
	result, err := readResponse(ctx, events, func() { t.Fatal("aborted without tool calls") }, nil, collectText(&texts))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(texts, "|") != "Hel|lo|, world" || result.Message.Content != "Hello, world" || result.FinishReason != "stop" || result.Provider != domain.CopilotProvider || result.Usage == nil || result.Usage.TotalTokens != 24 {
		t.Fatalf("texts %q, result %+v", texts, result)
	}
	if strings.Join(reasoning, "") != "thinking" {
		t.Fatalf("reasoning %q", reasoning)
	}
}

// Without deltas the final content is shown once; a message whose final
// content never came keeps what it streamed; content that does not extend
// the deltas shows nothing more.
func TestCopilotFallsBackToTheFinalMessage(t *testing.T) {
	var texts []string
	result, err := readResponse(context.Background(), fakeSession(&rpc.AssistantMessageData{MessageID: "m1", Content: "whole answer"}, &rpc.SessionIdleData{}), func() {}, nil, collectText(&texts))
	if err != nil || strings.Join(texts, "|") != "whole answer" || result.Message.Content != "whole answer" {
		t.Fatalf("texts %q, result %+v, %v", texts, result, err)
	}
	texts = nil
	result, err = readResponse(context.Background(), fakeSession(
		delta("m1", "first "), &rpc.AssistantMessageData{MessageID: "m1", Content: "first "},
		delta("m2", "second"), &rpc.SessionIdleData{},
	), func() {}, nil, collectText(&texts))
	if err != nil || strings.Join(texts, "|") != "first |second" || result.Message.Content != "first second" {
		t.Fatalf("texts %q, result %+v, %v", texts, result, err)
	}
	texts = nil
	result, err = readResponse(context.Background(), fakeSession(delta("m1", "draft"), &rpc.AssistantMessageData{MessageID: "m1", Content: "rewritten"}, &rpc.SessionIdleData{}), func() {}, nil, collectText(&texts))
	if err != nil || strings.Join(texts, "|") != "draft" || result.Message.Content != "rewritten" {
		t.Fatalf("texts %q, result %+v, %v", texts, result, err)
	}
}

// A message with tool requests ends the request: the SDK is aborted, the
// calls go back to AXLR and the streamed text is not shown again.
func TestCopilotReturnsToolRequestsAfterStreamedText(t *testing.T) {
	schema, _ := root.NewJSONObject([]byte(`{"type":"object"}`))
	defs := []root.ToolDefinition{{Name: "local_read", Description: "Read one file", Parameters: schema}}
	request := rpc.AssistantMessageToolRequest{ToolCallID: "one", Name: "local_read", Arguments: map[string]any{"path": "a.txt"}}
	aborted := 0
	var texts []string
	result, err := readResponse(context.Background(), fakeSession(delta("m1", "Reading"), &rpc.AssistantMessageData{MessageID: "m1", Content: "Reading", ToolRequests: []rpc.AssistantMessageToolRequest{request}}), func() { aborted++ }, defs, collectText(&texts))
	if err != nil || aborted != 1 || strings.Join(texts, "|") != "Reading" || result.FinishReason != "tool_calls" || len(result.Message.ToolCalls) != 1 || result.Message.Content != "Reading" {
		t.Fatalf("texts %q, result %+v, aborted %d, %v", texts, result, aborted, err)
	}
	request.Name = "shell"
	if _, err := readResponse(context.Background(), fakeSession(&rpc.AssistantMessageData{MessageID: "m1", ToolRequests: []rpc.AssistantMessageToolRequest{request}}), func() {}, defs, collectText(&texts)); err == nil {
		t.Fatal("an unavailable tool reached AXLR")
	}
}

// Cancelling mid-stream ends the request at once with no more text; a
// failing callback, a session error, a subagent and an empty answer end it
// with an error.
func TestCopilotStreamStopsOnCancellationAndErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	events := make(chan sdk.SessionEvent, 4)
	events <- sdk.SessionEvent{Data: delta("m1", "par")}
	var texts []string
	done := make(chan error, 1)
	go func() {
		_, err := readResponse(ctx, events, func() {}, nil, func(text root.Text) error {
			texts = append(texts, string(text))
			cancel()
			return nil
		})
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || strings.Join(texts, "|") != "par" {
			t.Fatalf("texts %q, %v", texts, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a cancelled stream kept waiting")
	}
	events <- sdk.SessionEvent{Data: delta("m1", "tial")}
	if _, err := readResponse(ctx, events, func() {}, nil, collectText(&texts)); !errors.Is(err, context.Canceled) || len(texts) != 1 {
		t.Fatalf("after cancel: %q, %v", texts, err)
	}
	refused := errors.New("console closed")
	if _, err := readResponse(context.Background(), fakeSession(delta("m1", "x")), func() {}, nil, func(root.Text) error { return refused }); !errors.Is(err, refused) {
		t.Fatalf("callback error: %v", err)
	}
	if _, err := readResponse(context.Background(), fakeSession(&rpc.AssistantMessageData{MessageID: "m1", Content: "x"}), func() {}, nil, func(root.Text) error { return refused }); !errors.Is(err, refused) {
		t.Fatalf("callback error on the final message: %v", err)
	}
	if _, err := readResponse(context.Background(), fakeSession(&rpc.SessionErrorData{ErrorType: "quota"}), func() {}, nil, collectText(&texts)); err == nil || !strings.Contains(err.Error(), "quota") {
		t.Fatalf("session error: %v", err)
	}
	agent := "sub"
	subagent := make(chan sdk.SessionEvent, 1)
	subagent <- sdk.SessionEvent{AgentID: &agent, Data: delta("m1", "x")}
	if _, err := readResponse(context.Background(), subagent, func() {}, nil, collectText(&texts)); err == nil {
		t.Fatal("a subagent event was accepted")
	}
	if _, err := readResponse(context.Background(), fakeSession(&rpc.SessionIdleData{}), func() {}, nil, collectText(&texts)); err == nil {
		t.Fatal("an empty answer was accepted")
	}
}

// The SDK session streams; it still runs no tool itself.
func TestSDKSessionStreams(t *testing.T) {
	if opts := sessionConfig("auto", "system", nil, []string{}); opts.Streaming == nil || !*opts.Streaming {
		t.Fatal("the session does not stream")
	}
}
