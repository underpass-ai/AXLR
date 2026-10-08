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
