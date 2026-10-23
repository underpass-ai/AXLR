package diagnostics

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestProviderActivityCoalescesReasoningAndDoesNotReopenAfterContent(t *testing.T) {
	var phases []domain.ProviderPhase
	ctx := application.WithProviderActivity(context.Background(), func(p domain.ProviderPhase) { phases = append(phases, p) })
	body := &streamBody{ctx: ctx}
	for i := 0; i < 2000; i++ {
		body.observeFrame([]byte(`{"choices":[{"delta":{"reasoning":"private model text"}}]}`))
	}
	body.observeFrame([]byte(`{"choices":[{"delta":{"tool_calls":[{"function":{"name":"tool"}}]}}]}`))
	body.observeFrame([]byte(`{"choices":[{"delta":{"content":"visible"}}]}`))
	body.observeFrame([]byte(`{"choices":[{"delta":{"reasoning_details":[{"text":"late private"}]}}]}`))
	if !reflect.DeepEqual(phases, []domain.ProviderPhase{domain.ProviderReasoning, domain.ProviderToolCall, domain.ProviderContent}) {
		t.Fatalf("phase sequence: %v", phases)
	}
}

func TestProviderActivityCancellationDoesNotNotifyNewTurn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ctx = application.WithProviderActivity(ctx, func(domain.ProviderPhase) { t.Fatal("notification after cancellation") })
	cancel()
	body := &streamBody{ctx: ctx}
	body.observeFrame([]byte(`{"choices":[{"delta":{"reasoning_content":"hidden"}}]}`))
	application.NotifyProviderActivity(nil, domain.ProviderContent)
	application.NotifyProviderActivity(context.Background(), domain.ProviderContent)
}

func TestOpaqueReasoningNotifiesWithoutExposingText(t *testing.T) {
	count := 0
	ctx := application.WithProviderActivity(context.Background(), func(phase domain.ProviderPhase) {
		count++
		if phase != domain.ProviderReasoning {
			t.Fatal(phase)
		}
	})
	body := &streamBody{ctx: ctx}
	body.observeFrame([]byte(`{"choices":[{"delta":{"reasoning_details":[{"type":"reasoning.encrypted","data":"opaque"}]}}]}`))
	if count != 1 {
		t.Fatal("opaque reasoning left model waiting")
	}
}

// Reasoning progress counts the reasoning bytes at the first frame and
// then every reasoningStep bytes, so a model that reasons for minutes shows
// it is still at work; each report carries the text streamed since the
// previous one.
func TestReasoningProgressCountsBytesInSteps(t *testing.T) {
	var told []int
	var text strings.Builder
	ctx := application.WithReasoningProgress(context.Background(), func(bytes int, delta string) {
		told = append(told, bytes)
		text.WriteString(delta)
	})
	body := &streamBody{ctx: ctx}
	frame := []byte(`{"choices":[{"delta":{"reasoning":"0123456789abcdef"}}]}`) // 16 bytes
	for range 70 {
		body.observeFrame(frame)
	}
	// 16 at once, then each time 512 more have come: 528, 1040.
	if !reflect.DeepEqual(told, []int{16, 528, 1040}) {
		t.Fatalf("progress %v", told)
	}
	// Reported text is the reasoning up to the last report, in order.
	if text.Len() != 1040 || !strings.HasPrefix(text.String(), "0123456789abcdef0123") {
		t.Fatalf("text %d bytes", text.Len())
	}
}

// The Anthropic Messages and OpenAI Responses streams carry no choices; their
// own events drive the same phases, reasoning progress and tool-call progress.
func TestNativeStreamsDriveTheSamePhases(t *testing.T) {
	for name, frames := range map[string][]string{
		"anthropic": {
			`{"type":"message_start","message":{"role":"assistant"}}`,
			`{"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}`,
			`{"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"private thought"}}`,
			`{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"read","input":{}}}`,
			`{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"path\":\"a\"}"}}`,
			`{"type":"content_block_start","index":2,"content_block":{"type":"text","text":""}}`,
			`{"type":"content_block_delta","index":2,"delta":{"type":"text_delta","text":"visible"}}`,
		},
		"responses": {
			`{"type":"response.output_item.added","output_index":0,"item":{"type":"reasoning","id":"rs_1"}}`,
			`{"type":"response.reasoning_summary_text.delta","output_index":0,"delta":"private thought"}`,
			`{"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","name":"read","call_id":"call_1"}}`,
			`{"type":"response.function_call_arguments.delta","output_index":1,"delta":"{\"path\":\"a\"}"}`,
			`{"type":"response.output_text.delta","output_index":2,"delta":"visible"}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			var phases []domain.ProviderPhase
			var reasoning []string
			var tool string
			var toolBytes int
			ctx := application.WithProviderActivity(context.Background(), func(phase domain.ProviderPhase) { phases = append(phases, phase) })
			ctx = application.WithReasoningProgress(ctx, func(_ int, text string) { reasoning = append(reasoning, text) })
			ctx = application.WithToolCallProgress(ctx, func(name string, bytes int) { tool, toolBytes = name, bytes })
			body := &streamBody{ctx: ctx}
			for _, frame := range frames {
				body.observeFrame([]byte(frame))
			}
			if len(phases) != 3 || phases[0] != domain.ProviderReasoning || phases[1] != domain.ProviderToolCall || phases[2] != domain.ProviderContent {
				t.Fatalf("phases %v", phases)
			}
			if strings.Join(reasoning, "") != "private thought" || tool != "read" || toolBytes != len(`{"path":"a"}`) {
				t.Fatalf("reasoning %q tool %q bytes %d", reasoning, tool, toolBytes)
			}
		})
	}
}
