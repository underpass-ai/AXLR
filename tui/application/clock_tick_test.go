package application

import (
	"runtime"
	"time"
)

// clockTick returns once this platform's clock has moved. Windows reads
// the time in ticks that can be milliseconds apart, so two readings around
// fast code are often the same instant and a sleep does not promise a new
// one: a test that needs time to have passed waits for it here. The
// release of v0.19.0 stopped on a 0s tool call, 15 November 2026.
func clockTick() {
	for start := time.Now(); time.Since(start) <= 0; {
		runtime.Gosched()
	}
}
