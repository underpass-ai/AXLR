package terminal

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/underpass-ai/AXLR/tui/application"
)

func TestFormatElapsed(t *testing.T) {
	for _, tc := range []struct {
		d    time.Duration
		want string
	}{{0, "0s"}, {42 * time.Second, "42s"}, {102 * time.Second, "1m42s"}, {3*time.Hour + 2*time.Minute + 5*time.Second, "3h02m"}} {
		if got := formatElapsed(tc.d); got != tc.want {
			t.Fatalf("%v: %q, want %q", tc.d, got, tc.want)
		}
	}
}

// The footer keeps the whole turn's clock next to the phase clock, which
// restarts at every request; a card keeps it and the turn's end stops it.
func TestTheFooterShowsTheWholeTurnClock(t *testing.T) {
	m := sized()
	defer m.zones.Close()
	m.Busy = true
	m.operationID = 1
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.cancel = cancel
	m = update(m, application.Event{Kind: application.EventStreamStart})
	m.turnStarted = time.Now().Add(-102 * time.Second)
	if got := footerText(m); !strings.Contains(got, "Waiting for model · 0s · t 1m42s") {
		t.Fatalf("footer: %q", got)
	}
	card := approvalModel(t)
	m = update(m, operationComplete{ID: 1, Session: *card.deps.Session})
	if m.turnStarted.IsZero() {
		t.Fatal("a card stopped the turn's clock")
	}
	if got := footerText(m); strings.Contains(got, "1m42s") {
		t.Fatalf("idle footer shows the clock: %q", got)
	}
	m.Busy, m.operationID = true, 2
	m = update(m, operationComplete{ID: 2, Session: *sized().deps.Session})
	if !m.turnStarted.IsZero() {
		t.Fatal("the turn's end kept its clock")
	}
}
