package domain

import (
	"strings"
	"testing"
)

// The memory engine's ids are single opaque tokens; a session instance
// refuses a malformed one and a predecessor that is not a session id.
func TestMemoryIDsAndTheInstanceThatCarriesThem(t *testing.T) {
	for _, good := range []string{"context_3ef9fb2088a79b17630a6cf71b6ca322", "agent_c7a85fa789ef9173d937401298f9d745", "ctx-1"} {
		if _, err := NewMemoryContextID(good); err != nil {
			t.Fatalf("%q refused: %v", good, err)
		}
		if _, err := NewMemoryAgentID(good); err != nil {
			t.Fatalf("%q refused: %v", good, err)
		}
	}
	for _, bad := range []string{"", "two words", "line\nbreak", strings.Repeat("x", 257)} {
		if _, err := NewMemoryContextID(bad); err == nil {
			t.Fatalf("context %q accepted", bad)
		}
		if _, err := NewMemoryAgentID(bad); err == nil {
			t.Fatalf("agent %q accepted", bad)
		}
	}
	if err := (SessionInstance{MemoryContext: "ctx_1", MemoryAgent: "agent_1"}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (SessionInstance{MemoryContext: "bad context"}).Validate(); err == nil {
		t.Fatal("a malformed context accepted")
	}
	if err := (SessionInstance{Instance: "axlr-session-x", MemoryAgent: "bad agent"}).Validate(); err == nil {
		t.Fatal("a malformed agent accepted")
	}
	if err := (SessionInstance{Predecessor: "0123456789abcdef0123456789abcdef"}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (SessionInstance{Predecessor: "not-a-session"}).Validate(); err == nil {
		t.Fatal("a malformed predecessor accepted")
	}
}
