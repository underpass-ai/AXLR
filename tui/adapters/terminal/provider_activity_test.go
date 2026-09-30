package terminal

import (
	"strings"
	"testing"

	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestProviderActivityShowsReasoningUntilVisibleContent(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m.Busy = true
	m.operationID = 1
	m = update(m, application.Event{Kind: application.EventStreamStart})
	m = update(m, application.Event{Kind: application.EventProviderActivity, ProviderPhase: domain.ProviderReasoning})
	if !m.providerWaiting || !strings.Contains(m.View().Content, "Model is reasoning") {
		t.Fatal("reasoning falsely looks like no provider activity")
	}
	m = update(m, application.Event{Kind: application.EventProviderActivity, ProviderPhase: domain.ProviderToolCall})
	if !strings.Contains(m.View().Content, "Model is preparing tools") {
		t.Fatal("tool arguments generation has no status")
	}
	m = update(m, application.Event{Kind: application.EventTextDelta, Text: "answer"})
	m = update(m, application.Event{Kind: application.EventProviderActivity, ProviderPhase: domain.ProviderReasoning})
	if m.providerWaiting || strings.Contains(m.View().Content, "Model is reasoning") {
		t.Fatal("late activity reopened waiting after answer text")
	}
}
