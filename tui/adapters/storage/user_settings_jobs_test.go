package storage

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestJobsSettings(t *testing.T) {
	base := DefaultUserSettings()
	if got := base.JobsConfiguration(); got.MaxActive != DefaultMaxActiveJobs || DefaultMaxActiveJobs != 2 {
		t.Fatalf("default = %+v", got)
	}
	var parsed UserSettings
	if err := json.Unmarshal([]byte(`{"jobs":{"max_active":4}}`), &parsed); err != nil {
		t.Fatal(err)
	}
	if err := parsed.Validate(); err != nil {
		t.Fatal(err)
	}
	if got := parsed.JobsConfiguration(); got.MaxActive != 4 || len(parsed.Extra) != 0 {
		t.Fatalf("parsed = %+v, extra %v", got, parsed.Extra)
	}
	encoded, err := json.Marshal(parsed)
	if err != nil || !strings.Contains(string(encoded), `"jobs":{"max_active":4}`) {
		t.Fatalf("encoded %s: %v", encoded, err)
	}
	for _, value := range []int{-1, 9} {
		settings := base
		settings.Jobs = &JobsSettings{MaxActive: value}
		if err := settings.Validate(); err == nil || !strings.Contains(err.Error(), "jobs.max_active must be between 1 and 8") {
			t.Fatalf("max_active %d: %v", value, err)
		}
	}
	base.Jobs = &JobsSettings{}
	if got := base.JobsConfiguration(); got.MaxActive != DefaultMaxActiveJobs {
		t.Fatalf("an empty section = %+v", got)
	}
}
