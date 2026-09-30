package diagnostics

import (
	"context"
	"reflect"
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
