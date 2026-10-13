package application

import "time"

// RepairSettings is what settings.json decides about self-repair, as the
// coordinator needs it.
type RepairSettings struct {
	// Repository is owner/name; About the KMP project about; Directory the
	// absolute directory the clones live in.
	Repository, About, Directory string
	// MaxAttempts bounds repair sessions per failure signature; zero means
	// DefaultMaxRepairAttempts.
	MaxAttempts int
	// MaxActive bounds the repair and improvement sessions active in the
	// registry, whoever started them; zero means DefaultMaxActiveJobs.
	MaxActive int
	// Watch bounds the merge queue's wait for one job's checks
	// (repair.watch_minutes); zero means the ceremony's default.
	Watch time.Duration
}
