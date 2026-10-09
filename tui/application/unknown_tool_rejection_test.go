package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

// A validation reason quotes the argument with %q, which keeps printable
// non-ASCII text: cutting it at 1024 bytes must not split a rune, or the
// rejection cannot be recorded and the call stays pending for good.
func TestRejectionReasonIsCutOnARuneBoundary(t *testing.T) {
	s := queued(t, call("a", "read"))
	prefix := "invalid tool invocation rejected: "
	reason := strings.Repeat("x", 1023-len(prefix)) + "ñññ"
	if err := rejectUnknownCall(context.Background(), &s, &memoryStore{}, nil, s.Pending()[0], errors.New(reason)); err != nil {
		t.Fatalf("rejection failed: %v", err)
	}
	if len(s.Pending()) != 0 {
		t.Fatalf("call still pending: %d", len(s.Pending()))
	}
	outcome := s.Export().Activity[0].Outcome
	if outcome == nil || !outcome.IsError {
		t.Fatalf("outcome = %+v", outcome)
	}
	if content := string(outcome.Content); !utf8.ValidString(content) || len(content) > 1024 || !strings.HasPrefix(content, prefix) {
		t.Fatalf("content (%d bytes, valid %v) = %q", len(content), utf8.ValidString(content), content)
	}
}
