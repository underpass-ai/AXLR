package storage

import (
	"encoding/json"
	"strings"
	"testing"
)

// notify is bell by default, osc or off when set, a known key, and any
// other value is refused.
func TestNotifySetting(t *testing.T) {
	for value, mode := range map[string]string{"": "bell", "bell": "bell", "osc": "osc", "off": "off"} {
		s := UserSettings{Language: "en", Theme: "auto", Icons: "safe", Notify: value}
		if err := s.Validate(); err != nil || s.NotifyMode() != mode {
			t.Fatalf("notify %q: %v %q", value, err, s.NotifyMode())
		}
	}
	if err := (UserSettings{Language: "en", Theme: "auto", Icons: "safe", Notify: "loud"}).Validate(); err == nil || !strings.Contains(err.Error(), `notify must be "bell", "osc" or "off"`) {
		t.Fatalf("loud: %v", err)
	}
	var s UserSettings
	if err := json.Unmarshal([]byte(`{"notify":"osc"}`), &s); err != nil || s.Notify != "osc" || len(s.Extra) != 0 {
		t.Fatalf("read: %+v %v", s, err)
	}
}
