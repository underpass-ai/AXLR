package application

import "time"

// SessionBundleMeta is what the bundle says about itself.
type SessionBundleMeta struct {
	ExportedAt time.Time
	// Host is the machine that exported it.
	Host  string
	Title string
	// About is the KMP about selected for the session, "" for the default.
	About string
}
