package service

import (
	"encoding/json"
	"strings"
	"testing"

	root "github.com/underpass-ai/AXLR/domain"
	"github.com/underpass-ai/AXLR/tui/application"
	"github.com/underpass-ai/AXLR/tui/domain"
)

func TestEventBatcherFlushesTextBeforeState(t *testing.T) {
	store, err := newEventStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{events: store}
	batch := newEventBatcher(server, "session", "operation", "actor", "request")
	for _, fragment := range []string{"hello", " ", "世界"} {
		if err := batch.Emit(application.Event{Kind: application.EventTextDelta, Text: root.Text(fragment)}); err != nil {
			t.Fatal(err)
		}
	}
	events, _, err := store.Read("session", 0)
	if err != nil || len(events) != 0 {
		t.Fatalf("text should remain buffered until the next event: events=%v err=%v", events, err)
	}
	if err := batch.Emit(application.Event{Kind: application.EventState, State: domain.StatusComplete}); err != nil {
		t.Fatal(err)
	}
	events, _, err = store.Read("session", 0)
	if err != nil || len(events) != 2 || events[0].Type != "text.delta" || events[1].Type != "turn.completed" {
		t.Fatalf("event order: events=%v err=%v", events, err)
	}
	var payload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil || payload.Text != "hello 世界" {
		t.Fatalf("text payload: %v %v", payload, err)
	}
}

func TestEventBatcherBoundsTextFrames(t *testing.T) {
	store, err := newEventStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	batch := newEventBatcher(&Server{events: store}, "session", "operation", "actor", "request")
	if err := batch.Emit(application.Event{Kind: application.EventTextDelta, Text: root.Text(strings.Repeat("x", 5000))}); err != nil {
		t.Fatal(err)
	}
	if err := batch.Flush(); err != nil {
		t.Fatal(err)
	}
	events, _, err := store.Read("session", 0)
	if err != nil || len(events) != 2 {
		t.Fatalf("expected two bounded frames: events=%d err=%v", len(events), err)
	}
	var all strings.Builder
	for _, event := range events {
		var payload struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(event.Payload, &payload); err != nil || len(payload.Text) > 4096 {
			t.Fatalf("invalid frame: %v %v", payload, err)
		}
		all.WriteString(payload.Text)
	}
	if all.String() != strings.Repeat("x", 5000) {
		t.Fatal("fragments were lost or reordered")
	}
}
