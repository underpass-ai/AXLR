package kmpstore

// Report says what Prepare did.
type Report struct {
	// Created is true when the directory did not hold a store yet.
	Created bool
	// Imported is the number of events restored from the bundle, -1 when
	// a bundle was imported but its receipt was unreadable, 0 without one.
	Imported int
	// ImportFailed is why the bundle was not imported; empty otherwise.
	// kmp-mcp initialises the store before it reads the bundle, so the
	// store stays and is configured and given the guide all the same, and
	// the person is told.
	ImportFailed string
	// Configured lists the configuration files copied from the user store.
	Configured []string
	// GuideSynced is true when the guide was written into the new store.
	GuideSynced bool
}
