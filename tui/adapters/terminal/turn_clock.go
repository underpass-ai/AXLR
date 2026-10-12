package terminal

import (
	"fmt"
	"time"
)

// The whole turn's clock. The footer's phase clock ("waiting for the
// model · 6s") restarts at every request and tool, so on 12 October 2026 a
// two-minute turn never showed two minutes. turnStarted is when the person
// sent the prompt, or when a decision or Ctrl+R took up a turn the console
// did not start; it holds while an approval card waits and clears when the
// turn ends.

// startTurnClock starts the clock of a prompt the person sent.
func (m *AppModel) startTurnClock() {
	m.turnStarted = time.Now()
}

// resumeTurnClock keeps a running turn's clock, or starts one for a turn
// taken up after a card or an interruption.
func (m *AppModel) resumeTurnClock() {
	if m.turnStarted.IsZero() {
		m.turnStarted = time.Now()
		// Its cost counts from here, as the turn's line will show.
		m.cost.beginTurn()
	}
}

// stopTurnClock ends the turn's clock and says how long the turn took,
// zero when none was running.
func (m *AppModel) stopTurnClock() time.Duration {
	if m.turnStarted.IsZero() {
		return 0
	}
	elapsed := time.Since(m.turnStarted)
	m.turnStarted = time.Time{}
	return elapsed
}

// turnClock is the footer's "⏱ 1m42s" while a turn runs, "" otherwise.
func (m AppModel) turnClock() string {
	if m.turnStarted.IsZero() || !m.Busy || !m.turnRunning() {
		return ""
	}
	return m.Theme.Icon("timer") + " " + formatElapsed(time.Since(m.turnStarted))
}

// formatElapsed is "42s", "1m42s" or "1h02m".
func formatElapsed(d time.Duration) string {
	seconds := int(d.Seconds())
	switch {
	case seconds < 60:
		return fmt.Sprintf("%ds", seconds)
	case seconds < 3600:
		return fmt.Sprintf("%dm%02ds", seconds/60, seconds%60)
	default:
		return fmt.Sprintf("%dh%02dm", seconds/3600, seconds/60%60)
	}
}
