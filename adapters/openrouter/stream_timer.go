package openrouter

import "time"

// streamTimer is a stream's inactivity or maximum-duration timer; tests fire
// it by hand so no assertion depends on runner speed.
type streamTimer interface {
	Stop() bool
	Reset(time.Duration) bool
}

// afterFunc starts a streamTimer that calls f once d has passed.
type afterFunc func(d time.Duration, f func()) streamTimer

func realAfterFunc(d time.Duration, f func()) streamTimer { return time.AfterFunc(d, f) }
