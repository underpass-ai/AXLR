package storage

// CeremonySettings is the ceremonies section. Profile is "auto" (the
// default: compact when the session model's known window is at most
// CompactWindowTokens), "standard" or "compact".
type CeremonySettings struct {
	Profile string `json:"profile,omitempty"`
}

// CompactWindowTokens is the largest window the auto profile treats as small.
const CompactWindowTokens = 65536
