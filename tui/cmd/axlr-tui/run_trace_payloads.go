package main

import (
	"flag"

	"github.com/underpass-ai/AXLR/tui/adapters/storage"
)

// capturePayloads decides whether this launch stores request and response
// bodies. They hold whole transcripts: on 9 October 2026 the default logs
// directory held 295 MB of them, 74 MB from a single run, so capture is off
// unless trace_payloads asks for it, and --trace-payloads, when given,
// decides for this launch either way.
func capturePayloads(flags *flag.FlagSet, flagValue bool, settings storage.UserSettings) bool {
	capture := settings.TracePayloads
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "trace-payloads" {
			capture = flagValue
		}
	})
	return capture
}
