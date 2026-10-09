package storage

import (
	"encoding/json"
	"testing"
)

func TestSharedEnginesAreOptIn(t *testing.T) {
	var settings UserSettings
	if err := json.Unmarshal([]byte(`{}`), &settings); err != nil || settings.SharedEngines() {
		t.Fatalf("default shared = %v, %v", settings.SharedEngines(), err)
	}
	if err := json.Unmarshal([]byte(`{"engines":{"shared":true}}`), &settings); err != nil || !settings.SharedEngines() || len(settings.Extra) != 0 {
		t.Fatalf("configured shared = %v, extra %v, %v", settings.SharedEngines(), settings.Extra, err)
	}
}
