package storage

import "testing"

func TestCopilotModelValidation(t *testing.T) {
	model := LocalModel{ID: "copilot/auto", API: APICopilot, Model: "auto", ContextTokens: 64000}
	if err := model.validate(); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []LocalModel{
		{ID: "copilot/auto", API: APICopilot, ContextTokens: 64000},
		{ID: "copilot/auto", API: APICopilot, Model: "auto", URL: "https://example.com", ContextTokens: 64000},
		{ID: "copilot/auto", API: APICopilot, Model: "auto", APIKeyEnv: "SECRET", ContextTokens: 64000},
	} {
		if err := bad.validate(); err == nil {
			t.Fatalf("accepted invalid configuration: %+v", bad)
		}
	}
}
