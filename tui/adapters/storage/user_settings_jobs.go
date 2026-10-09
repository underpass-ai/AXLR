package storage

import "fmt"

// JobsSettings is the jobs section of settings.json: MaxActive bounds the
// repair and improvement sessions active at once across the consoles that
// share the registry, whoever started them (default 2, from 1 to 8).
type JobsSettings struct {
	MaxActive int `json:"max_active,omitempty"`
}

// DefaultMaxActiveJobs and maxActiveJobs bound jobs.max_active. Each job
// runs a model session and its checks in its own clone; eight at once were
// already more than the machine held on 9 October 2026.
const (
	DefaultMaxActiveJobs = 2
	maxActiveJobs        = 8
)

// JobsConfiguration is the jobs section with defaults applied.
func (s UserSettings) JobsConfiguration() JobsSettings {
	jobs := JobsSettings{MaxActive: DefaultMaxActiveJobs}
	if s.Jobs != nil && s.Jobs.MaxActive > 0 {
		jobs.MaxActive = s.Jobs.MaxActive
	}
	return jobs
}

func (j *JobsSettings) validate() error {
	if j == nil {
		return nil
	}
	if j.MaxActive < 0 || j.MaxActive > maxActiveJobs {
		return fmt.Errorf("settings jobs.max_active must be between 1 and %d", maxActiveJobs)
	}
	return nil
}
