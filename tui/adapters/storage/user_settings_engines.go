package storage

// EngineSettings is the engines section: Shared runs one KMP and one MADE
// process per identical launch for every console of the user, instead of
// one per console (default false).
type EngineSettings struct {
	Shared bool `json:"shared,omitempty"`
	// Required refuses to start without KMP and MADE connected (decision
	// A2); a complete settings file writes it true.
	Required bool `json:"required,omitempty"`
}

// EnginesRequired reports whether the console refuses to start without
// both engines.
func (s UserSettings) EnginesRequired() bool { return s.Engines != nil && s.Engines.Required }

// SharedEngines reports whether consoles share their engine processes.
func (s UserSettings) SharedEngines() bool { return s.Engines != nil && s.Engines.Shared }
