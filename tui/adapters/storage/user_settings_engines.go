package storage

// EngineSettings is the engines section: Shared runs one KMP and one MADE
// process per identical launch for every console of the user, instead of
// one per console (default false).
type EngineSettings struct {
	Shared bool `json:"shared,omitempty"`
}

// SharedEngines reports whether consoles share their engine processes.
func (s UserSettings) SharedEngines() bool { return s.Engines != nil && s.Engines.Shared }
