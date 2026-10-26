package storage

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSettingsRoutingIsTheContractsObject(t *testing.T) {
	var settings UserSettings
	if err := json.Unmarshal([]byte(`{"model": "qwen-local", "routing": {"candidates": ["qwen-local", "anthropic/claude-haiku-5.5"], "objective": "cost", "allow_remote_providers": false}}`), &settings); err != nil {
		t.Fatal(err)
	}
	if err := settings.Validate(); err != nil {
		t.Fatal(err)
	}
	routing, err := settings.ExecutionRouting()
	if err != nil || routing == nil || routing.AllowsRemote() || len(routing.Candidates()) != 2 {
		t.Fatalf("%+v %v", routing, err)
	}
	if routing, err := DefaultUserSettings().ExecutionRouting(); routing != nil || err != nil {
		t.Fatalf("no routing read as %+v %v", routing, err)
	}
	settings.Routing = json.RawMessage(`{"candidates": ["a"], "objective": "cheapest"}`)
	if err := settings.Validate(); err == nil || !strings.Contains(err.Error(), "settings routing") {
		t.Fatalf("an invalid routing validated: %v", err)
	}
}
