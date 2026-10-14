package ceremonyhost

import "testing"

// A message that already names its code is not prefixed twice.
func TestRefusalNamesItsCodeOnce(t *testing.T) {
	if got := (&refusal{Code: "conflict", Message: "conflict: claim changed"}).Error(); got != "conflict: claim changed" {
		t.Fatalf("%q", got)
	}
	if got := (&refusal{Code: "conflict", Message: "claim changed"}).Error(); got != "conflict: claim changed" {
		t.Fatalf("%q", got)
	}
}
