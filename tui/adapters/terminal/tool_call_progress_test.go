package terminal

import (
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestFooterNamesTheStreamingToolCall(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m.Busy = true
	m.operationID = 1
	m = update(m, application.Event{Kind: application.EventStreamStart})
	m = update(m, application.Event{Kind: application.EventProviderActivity, ProviderPhase: domain.ProviderToolCall})
	if !strings.Contains(m.View().Content, "Model is preparing tools") {
		t.Fatal("tool call preparation without a name lost its generic status")
	}
	m = update(m, application.Event{Kind: application.EventToolCallProgress, ToolCallName: "local_write", ToolCallBytes: 4300})
	if !strings.Contains(m.View().Content, "Model is preparing local_write · 4.2\u00a0KB") {
		t.Fatalf("footer does not name the streaming tool call:\n%s", m.View().Content)
	}
	m = update(m, application.Event{Kind: application.EventTextDelta, Text: "answer"})
	if strings.Contains(m.View().Content, "preparing local_write") {
		t.Fatal("tool call detail survived visible answer text")
	}
}

// On 14 October 2026 glm-5.3-flash reasoned for four to five minutes and
// the footer said only "Model is reasoning" and a clock: it now counts what
// the model has reasoned.
func TestFooterCountsWhatTheModelHasReasoned(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m.Busy = true
	m.operationID = 1
	m = update(m, application.Event{Kind: application.EventStreamStart})
	m = update(m, application.Event{Kind: application.EventProviderActivity, ProviderPhase: domain.ProviderReasoning})
	m = update(m, application.Event{Kind: application.EventReasoningProgress, ReasoningBytes: 3200})
	if !strings.Contains(m.View().Content, "Model is reasoning · 3.1 KB") {
		t.Fatalf("footer does not count the reasoning:\n%s", m.View().Content)
	}
	if bar := (StatusBar{Waiting: true, Phase: domain.ProviderReasoning, ReasoningBytes: 3200}).View(200); !strings.Contains(bar, "Model is reasoning · 3.1 KB") {
		t.Fatalf("panel status %q", bar)
	}
	m = update(m, application.Event{Kind: application.EventStreamStart})
	if strings.Contains(m.View().Content, "3.1 KB") {
		t.Fatal("the count survived into the next request")
	}
}
