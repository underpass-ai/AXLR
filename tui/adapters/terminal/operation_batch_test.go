package terminal

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/underpass-ai/AXLR/tui/application"
)

func TestReadOperationCoalescesQueuedTextAndPreservesCompletion(t *testing.T) {
	ch := make(chan tea.Msg, 8)
	ch <- application.Event{Kind: application.EventTextDelta, Text: "one"}
	ch <- application.Event{Kind: application.EventTextDelta, Text: " two"}
	ch <- application.Event{Kind: application.EventTextDelta, Text: " three"}
	ch <- operationComplete{ID: 9}
	got := readOperation(ch)()
	batch, ok := got.(operationBatch)
	if !ok || len(batch.Messages) != 2 {
		t.Fatalf("queued messages were not coalesced: %#v", got)
	}
	text, ok := batch.Messages[0].(application.Event)
	if !ok || text.Kind != application.EventTextDelta || text.Text != "one two three" {
		t.Fatalf("text changed: %#v", batch.Messages[0])
	}
	if _, ok := batch.Messages[1].(operationComplete); !ok {
		t.Fatalf("completion lost or reordered: %#v", batch.Messages[1])
	}
}

func TestReadOperationBoundsBatchAndDeliversLaterCompletion(t *testing.T) {
	ch := make(chan tea.Msg, 301)
	for range 300 {
		ch <- application.Event{Kind: application.EventTextDelta, Text: "x"}
	}
	ch <- operationComplete{ID: 12}
	var text strings.Builder
	completed := 0
	for completed == 0 {
		msg := readOperation(ch)()
		var messages []tea.Msg
		if batch, ok := msg.(operationBatch); ok {
			messages = batch.Messages
		} else {
			messages = []tea.Msg{msg}
		}
		for _, item := range messages {
			switch v := item.(type) {
			case application.Event:
				text.WriteString(string(v.Text))
			case operationComplete:
				completed++
			}
		}
	}
	if text.String() != strings.Repeat("x", 300) || completed != 1 {
		t.Fatalf("lost or duplicated events: bytes=%d completions=%d", text.Len(), completed)
	}
}

func TestReadOperationFlushesSingleSlowDelta(t *testing.T) {
	ch := make(chan tea.Msg, 1)
	ch <- application.Event{Kind: application.EventTextDelta, Text: "visible"}
	result := make(chan tea.Msg, 1)
	go func() { result <- readOperation(ch)() }()
	select {
	case msg := <-result:
		if event, ok := msg.(application.Event); !ok || event.Text != "visible" {
			t.Fatalf("first fragment changed: %#v", msg)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("single fragment did not become visible promptly")
	}
}

func TestReadOperationPreservesStateBoundaryBetweenTextRuns(t *testing.T) {
	ch := make(chan tea.Msg, 8)
	ch <- application.Event{Kind: application.EventTextDelta, Text: "before"}
	ch <- application.Event{Kind: application.EventState}
	ch <- application.Event{Kind: application.EventTextDelta, Text: "after"}
	ch <- operationComplete{ID: 1}
	batch := readOperation(ch)().(operationBatch)
	if len(batch.Messages) != 4 || batch.Messages[0].(application.Event).Text != "before" || batch.Messages[1].(application.Event).Kind != application.EventState || batch.Messages[2].(application.Event).Text != "after" {
		t.Fatalf("event order changed: %#v", batch.Messages)
	}
}

func TestReadOperationKeepsEmptyDeltaSoNextReadRuns(t *testing.T) {
	ch := make(chan tea.Msg, 2)
	ch <- application.Event{Kind: application.EventTextDelta}
	first := readOperation(ch)()
	if event, ok := first.(application.Event); !ok || event.Kind != application.EventTextDelta {
		t.Fatalf("empty delta was dropped: %#v", first)
	}
	ch <- operationComplete{ID: 3}
	if _, ok := readOperation(ch)().(operationComplete); !ok {
		t.Fatal("completion after empty delta was lost")
	}
}
